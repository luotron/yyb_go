package crypto

import "testing"

func TestBuildActivitySignatureMatchesHAR(t *testing.T) {
	const signKey = "1154b7aa0d768d7bde8714d9b43065c9"
	tests := []struct {
		timestamp string
		nonce     string
		sign      string
	}{
		{"1785266234493", "02aa1456faf6f26a", "638421bf69819c6021bb78bfe014900fbd849b10564dab60da0ac0bd4b41b04f"},
		{"1785266267037", "efbe1accbef45a1e", "3cb98d55ba34030da6b225c836c16f6783cd35cda505475907d7cfaa21015577"},
		{"1785266291451", "3dcd39111d5d66ff", "73341cb47e9b2bfc51cd462d0176cd7f18d75fdfad62d996b32be3f9e3aba2ae"},
	}
	for _, test := range tests {
		result, err := BuildActivitySignature(signKey, test.timestamp, test.nonce)
		if err != nil {
			t.Fatalf("BuildActivitySignature() error = %v", err)
		}
		if result.Sign != test.sign {
			t.Fatalf("timestamp=%s sign=%s，期望 %s", test.timestamp, result.Sign, test.sign)
		}
	}
}

func TestGenerateActivitySignatureIsSelfConsistent(t *testing.T) {
	result, err := GenerateActivitySignature("1154b7aa0d768d7bde8714d9b43065c9", "TOKEN")
	if err != nil {
		t.Fatalf("GenerateActivitySignature() error = %v", err)
	}
	if len(result.SignNonce) != 16 || len(result.Sign) != 64 || result.SignTimestamp == "" {
		t.Fatalf("GenerateActivitySignature() = %#v", result)
	}
	verified, err := BuildActivitySignature("1154b7aa0d768d7bde8714d9b43065c9", result.SignTimestamp, result.SignNonce)
	if err != nil || verified.Sign != result.Sign {
		t.Fatalf("生成结果复算失败: %#v, %v", verified, err)
	}
}
