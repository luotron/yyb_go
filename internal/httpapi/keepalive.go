package httpapi

import (
	"context"
	"fmt"
	"log"
	"strings"
	"sync"
	"time"

	"yyb_go/internal/protocol"
	"yyb_go/internal/store"
)

const keepAliveRetryBackoff = 5 * time.Minute

func (a *App) startKeepAlive() {
	if a.cfg.KeepAliveInterval <= 0 {
		return
	}
	ctx, cancel := context.WithCancel(context.Background())
	a.keepAliveCancel = cancel
	a.keepAliveDone = make(chan struct{})
	log.Printf("keepalive: enabled interval=%s refresh_ahead=%s", a.cfg.KeepAliveInterval, a.cfg.KeepAliveAhead)
	go func() {
		defer close(a.keepAliveDone)
		a.keepAliveLoop(ctx)
	}()
}

func (a *App) keepAliveLoop(ctx context.Context) {
	a.refreshDueAccounts(ctx)
	ticker := time.NewTicker(a.cfg.KeepAliveInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			a.refreshDueAccounts(ctx)
		}
	}
}

func (a *App) refreshDueAccounts(ctx context.Context) {
	accounts, err := a.db.ListAccounts(ctx)
	if err != nil {
		if ctx.Err() == nil {
			log.Printf("keepalive: list accounts: %v", err)
		}
		return
	}
	const maxWorkers = 4
	sem := make(chan struct{}, maxWorkers)
	var wg sync.WaitGroup
accountsLoop:
	for _, acc := range accounts {
		if ctx.Err() != nil {
			break accountsLoop
		}
		if accountStatus(acc) == "expired" {
			continue
		}
		if a.keepAliveShouldSkip(ctx, acc, time.Now()) {
			continue
		}
		select {
		case sem <- struct{}{}:
		case <-ctx.Done():
			break accountsLoop
		}
		wg.Add(1)
		go func(acc *store.WechatAccount) {
			defer wg.Done()
			defer func() { <-sem }()
			accountCtx, cancel := context.WithTimeout(ctx, keepAliveAccountTimeout(a.cfg.RequestTimeout))
			defer cancel()
			_, refreshed, err := a.refreshAccount(accountCtx, acc, false)
			if err != nil {
				a.setKeepAliveRetry(acc.ID, time.Now().Add(keepAliveRetryBackoff))
				if ctx.Err() == nil {
					log.Printf("keepalive: account id=%d refresh failed: %v", acc.ID, err)
				}
				return
			}
			a.clearKeepAliveRetry(acc.ID)
			if refreshed {
				log.Printf("keepalive: account id=%d credentials renewed", acc.ID)
			}
		}(acc)
	}
	wg.Wait()
}

func (a *App) keepAliveShouldSkip(ctx context.Context, acc *store.WechatAccount, now time.Time) bool {
	if !credentialsDueForRefresh(protocol.CredentialsFromMap(acc.Credentials), now, a.cfg.KeepAliveAhead) {
		return true
	}
	a.keepAliveRetryMu.Lock()
	defer a.keepAliveRetryMu.Unlock()
	retryAt, ok := a.keepAliveRetryAt[acc.ID]
	if !ok {
		return false
	}
	if !now.Before(retryAt) {
		delete(a.keepAliveRetryAt, acc.ID)
		return false
	}
	return true
}

func (a *App) setKeepAliveRetry(accountID int64, retryAt time.Time) {
	a.keepAliveRetryMu.Lock()
	a.keepAliveRetryAt[accountID] = retryAt
	a.keepAliveRetryMu.Unlock()
}

func (a *App) clearKeepAliveRetry(accountID int64) {
	a.keepAliveRetryMu.Lock()
	delete(a.keepAliveRetryAt, accountID)
	a.keepAliveRetryMu.Unlock()
}

func keepAliveAccountTimeout(requestTimeout time.Duration) time.Duration {
	if requestTimeout <= 0 {
		return 30 * time.Second
	}
	if timeout := requestTimeout * 4; timeout > 30*time.Second {
		return timeout
	}
	return 30 * time.Second
}

// refreshAccount 按需刷新账号凭证。force 为 true 时跳过到期检查强制刷新。
// 返回最终状态、是否实际刷新、以及错误（状态为 alive 时错误为 nil）。
func (a *App) refreshAccount(ctx context.Context, acc *store.WechatAccount, force bool) (string, bool, error) {
	lock := a.refreshLockFor(acc.ID)
	lock.Lock()
	defer lock.Unlock()

	latest, err := a.db.GetAccount(ctx, acc.ID)
	if err != nil {
		return "unknown", false, err
	}
	if accountStatus(latest) == "expired" {
		return "expired", false, nil
	}
	if latest.Credentials == nil {
		err = fmt.Errorf("credentials are missing")
		if setErr := a.db.SetAccountStatus(ctx, latest.ID, "unknown"); setErr != nil {
			err = fmt.Errorf("%v; update status: %w", err, setErr)
		}
		return "unknown", false, err
	}

	creds := protocol.CredentialsFromMap(latest.Credentials)
	if !force && !credentialsDueForRefresh(creds, time.Now(), a.cfg.KeepAliveAhead) {
		return accountStatus(latest), false, nil
	}

	result, err := a.qr.RefreshLoginBuffer(ctx, creds)
	if err != nil {
		status := refreshFailureStatus(accountStatus(latest), creds, err, time.Now())
		if setErr := a.db.SetAccountStatus(ctx, latest.ID, status); setErr != nil {
			err = fmt.Errorf("%v; update status: %w", err, setErr)
		}
		return status, false, err
	}
	if err = a.db.SetAccountCredential(ctx, latest.ID, result.LoginBuffer, result.Credentials.ToMap()); err != nil {
		return "expired", false, err
	}
	if err = a.db.SetAccountStatus(ctx, latest.ID, "alive"); err != nil {
		return "expired", false, err
	}
	if avatar := a.resolveAvatar(ctx, latest.OpenID, latest.UserInfo); avatar != "" {
		_ = a.db.SetAccountProfile(ctx, latest.ID, latest.Nickname, &avatar, latest.UserInfo)
	}
	return "alive", true, nil
}

func (a *App) refreshLockFor(accountID int64) *sync.Mutex {
	a.refreshLocksMu.Lock()
	defer a.refreshLocksMu.Unlock()
	if lock := a.refreshLocks[accountID]; lock != nil {
		return lock
	}
	lock := &sync.Mutex{}
	a.refreshLocks[accountID] = lock
	return lock
}

func refreshFailureStatus(current string, creds protocol.LoginBufferCredentials, err error, now time.Time) string {
	if definitiveCredentialFailure(err) {
		return "expired"
	}
	if creds.ExpiresAt > now.Unix() {
		return current
	}
	return "unknown"
}

func definitiveCredentialFailure(err error) bool {
	return err != nil && definitiveRefreshMessage(err.Error())
}

func definitiveRefreshMessage(raw string) bool {
	message := strings.ToLower(strings.TrimSpace(raw))
	if strings.Contains(message, "missing refresh token") {
		return true
	}
	if strings.Contains(message, "42007") && strings.Contains(message, "refresh_token") {
		return true
	}
	if strings.Contains(message, "40188") && strings.Contains(message, "invalid scope") {
		return true
	}
	invalid := strings.Contains(message, "invalid") || strings.Contains(message, "expired") ||
		strings.Contains(message, "expire") || strings.Contains(message, "无效") ||
		strings.Contains(message, "过期") || strings.Contains(message, "失效")
	token := strings.Contains(message, "token") || strings.Contains(message, "登录") ||
		strings.Contains(message, "凭证") || strings.Contains(message, "授权")
	relogin := strings.Contains(message, "relogin") || strings.Contains(message, "re-login") ||
		strings.Contains(message, "重新登录") || strings.Contains(message, "重新授权")
	return relogin || (invalid && token)
}

func credentialsDueForRefresh(creds protocol.LoginBufferCredentials, now time.Time, ahead time.Duration) bool {
	return creds.ExpiresAt <= 0 || now.Add(ahead).Unix() >= creds.ExpiresAt
}

func accountStatus(acc *store.WechatAccount) string {
	if acc.Status == nil || *acc.Status == "" {
		return "unknown"
	}
	return *acc.Status
}
