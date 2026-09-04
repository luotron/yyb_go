package httpapi

func newOpenAPISpec() map[string]any {
	return map[string]any{
		"openapi": "3.0.3",
		"info": map[string]any{
			"title":       "YYB Go 接口文档",
			"description": "用于微信扫码登录、账号管理和 wxapp 接口调用的 API。",
			"version":     "1.0.0",
		},
		"servers": []map[string]any{
			{"url": "/"},
		},
		"tags": []map[string]any{
			{"name": "health", "description": "服务健康检查"},
			{"name": "qr", "description": "微信扫码登录"},
			{"name": "accounts", "description": "已保存的微信账号"},
			{"name": "wxapp", "description": "wxapp 业务接口调用"},
			{"name": "calls", "description": "按账号与功能名统一调用小程序能力"},
			{"name": "activity", "description": "活动请求签名"},
			{"name": "scripts", "description": "用户 Python 脚本运行与定时调度"},
		},
		"paths": map[string]any{
			"/health": map[string]any{
				"get": openAPIOperation(
					[]string{"health"},
					"检查服务状态",
					nil,
					nil,
					defaulted(map[string]any{
						"200": jsonResponse("服务正常。", refSchema("HealthResponse")),
					}),
				),
			},
			"/ready": map[string]any{
				"get": openAPIOperation(
					[]string{"health"},
					"检查服务是否已可处理业务请求",
					nil,
					nil,
					defaulted(map[string]any{
						"200": jsonResponse("服务已就绪。", refSchema("ReadyResponse")),
					}),
				),
			},
			"/qr": map[string]any{
				"post": openAPIOperation(
					[]string{"qr"},
					"创建扫码登录会话",
					[]map[string]any{
						boolQueryParam("as_base64", "是否同时返回二维码图片的 data URI。"),
					},
					nil,
					defaulted(map[string]any{
						"200": jsonResponse("二维码会话创建成功。", refSchema("QRCreateResponse")),
					}),
				),
			},
			"/qr/{session_id}/image": map[string]any{
				"get": openAPIOperation(
					[]string{"qr"},
					"获取二维码图片",
					[]map[string]any{pathStringParam("session_id", "二维码会话 ID。")},
					nil,
					defaulted(map[string]any{
						"200": imageResponse("二维码图片。"),
					}),
				),
			},
			"/qr/{session_id}/poll": map[string]any{
				"get": openAPIOperation(
					[]string{"qr"},
					"轮询扫码登录状态",
					[]map[string]any{pathStringParam("session_id", "二维码会话 ID。")},
					nil,
					defaulted(map[string]any{
						"200": jsonResponse("当前扫码状态。", refSchema("QRPollResponse")),
					}),
				),
			},
			"/qr/{session_id}/confirm": map[string]any{
				"post": openAPIOperation(
					[]string{"qr"},
					"确认已授权的扫码会话并保存账号",
					[]map[string]any{pathStringParam("session_id", "二维码会话 ID。")},
					nil,
					defaulted(map[string]any{
						"200": jsonResponse("已保存的账号信息。", refSchema("AccountPublic")),
					}),
				),
			},
			"/accounts": map[string]any{
				"get": openAPIOperation(
					[]string{"accounts"},
					"获取账号列表",
					nil,
					nil,
					defaulted(map[string]any{
						"200": jsonResponse("已保存的账号列表。", arraySchema(refSchema("AccountPublic"))),
					}),
				),
				"delete": openAPIOperation(
					[]string{"accounts"},
					"删除账号",
					[]map[string]any{queryStringParam("ref", "账号 ID、UIN 或 openid。", true)},
					nil,
					defaulted(map[string]any{
						"200": jsonResponse("删除结果。", refSchema("DeleteAccountResponse")),
					}),
				),
			},
			"/accounts/refresh": map[string]any{
				"post": openAPIOperation(
					[]string{"accounts"},
					"刷新账号存活状态",
					nil,
					jsonOptionalRequestBody(refSchema("AccountRefRequest")),
					defaulted(map[string]any{
						"200": jsonResponse("刷新结果。未传 ref 时返回数组。", refSchema("RefreshResponse")),
					}),
				),
			},
			"/accounts/resync": map[string]any{
				"post": openAPIOperation(
					[]string{"accounts"},
					"重新同步账号资料",
					nil,
					jsonOptionalRequestBody(refSchema("AccountRefRequest")),
					defaulted(map[string]any{
						"200": jsonResponse("同步后的账号信息。未传 ref 时返回数组。", refSchema("ResyncResponse")),
					}),
				),
			},
			"/accounts/avatar": map[string]any{
				"get": openAPIOperation(
					[]string{"accounts"},
					"获取账号头像",
					[]map[string]any{queryStringParam("ref", "账号 ID、UIN 或 openid。", true)},
					nil,
					defaulted(map[string]any{
						"200": imageResponse("头像图片。"),
						"302": map[string]any{"description": "跳转到远程头像地址。"},
					}),
				),
			},
			"/features": map[string]any{
				"get": openAPIOperation(
					[]string{"calls"},
					"获取可调用功能列表",
					[]map[string]any{boolQueryParam("only_enabled", "仅返回已启用功能。")},
					nil,
					defaulted(map[string]any{
						"200": jsonResponse("功能列表。", arraySchema(refSchema("FeatureDefinition"))),
					}),
				),
			},
			"/activity/sign": map[string]any{
				"post": openAPIOperation(
					[]string{"activity"},
					"生成活动请求 signTimestamp、signNonce 与 sign",
					nil,
					jsonRequestBody(refSchema("ActivitySignRequest")),
					defaulted(map[string]any{
						"200": jsonResponse("活动签名结果。", refSchema("ActivitySignResponse")),
					}),
				),
			},
			"/accounts/{ref}/call": map[string]any{
				"post": openAPIOperation(
					[]string{"calls"},
					"按账号调用小程序功能",
					[]map[string]any{pathStringParam("ref", "账号 ID、UIN 或 openid。")},
					jsonRequestBody(refSchema("CallFeatureRequest")),
					defaulted(map[string]any{
						"200": jsonResponse("功能调用结果。", refSchema("CallFeatureResponse")),
					}),
				),
			},
			"/wxapp/getCode": map[string]any{
				"post": openAPIOperation(
					[]string{"wxapp"},
					"获取小程序code",
					nil,
					jsonRequestBody(refSchema("WxappRequest")),
					defaulted(map[string]any{
						"200": jsonResponse("getCode 调用结果。", refSchema("WxappResponse")),
					}),
				),
			},
			"/wxapp/getPhoneNumber": map[string]any{
				"post": openAPIOperation(
					[]string{"wxapp"},
					"获取手机号",
					nil,
					jsonRequestBody(refSchema("WxappRequest")),
					defaulted(map[string]any{
						"200": jsonResponse("getPhoneNumber 调用结果。", refSchema("WxappResponse")),
					}),
				),
			},
			"/wxapp/operateWxData": map[string]any{
				"post": openAPIOperation(
					[]string{"wxapp"},
					"小程序云函数",
					nil,
					jsonRequestBody(refSchema("OperateWXDataRequest")),
					defaulted(map[string]any{
						"200": jsonResponse("operateWxData 调用结果。", refSchema("WxappResponse")),
					}),
				),
			},
			"/wxapp/getHostSign": map[string]any{
				"post": openAPIOperation(
					[]string{"wxapp"},
					"获取 verifyplugin HostSign",
					nil,
					jsonRequestBody(refSchema("HostSignRequest")),
					defaulted(map[string]any{
						"200": jsonResponse("getHostSign 调用结果。", refSchema("WxappResponse")),
					}),
				),
			},
			"/scripts": map[string]any{
				"get": openAPIOperation(
					[]string{"scripts"},
					"列出用户脚本及运行状态",
					nil,
					nil,
					defaulted(map[string]any{
						"200": jsonResponse("脚本列表与运行器信息。", refSchema("ScriptListResponse")),
					}),
				),
				"post": openAPIOperation(
					[]string{"scripts"},
					"上传 .py 脚本（multipart/form-data 字段 file）",
					[]map[string]any{boolQueryParam("overwrite", "已存在同名脚本时是否覆盖。")},
					map[string]any{
						"required": true,
						"content": map[string]any{
							"multipart/form-data": map[string]any{
								"schema": refSchema("ScriptUploadRequest"),
							},
						},
					},
					defaulted(map[string]any{
						"200": jsonResponse("上传结果。", refSchema("ScriptUploadResponse")),
					}),
				),
			},
			"/scripts/{name}/run": map[string]any{
				"post": openAPIOperation(
					[]string{"scripts"},
					"立即运行脚本",
					[]map[string]any{pathStringParam("name", "脚本文件名，如 demo.py。")},
					jsonRequestBody(refSchema("ScriptRunRequest")),
					defaulted(map[string]any{
						"200": jsonResponse("脚本状态。", refSchema("ScriptInfo")),
					}),
				),
			},
			"/scripts/{name}/stop": map[string]any{
				"post": openAPIOperation(
					[]string{"scripts"},
					"停止正在运行的脚本",
					[]map[string]any{pathStringParam("name", "脚本文件名。")},
					nil,
					defaulted(map[string]any{
						"200": jsonResponse("停止结果。", refSchema("ScriptStopResponse")),
					}),
				),
			},
			"/scripts/{name}/logs": map[string]any{
				"get": openAPIOperation(
					[]string{"scripts"},
					"读取脚本运行日志",
					[]map[string]any{pathStringParam("name", "脚本文件名。")},
					nil,
					defaulted(map[string]any{
						"200": jsonResponse("日志内容与运行状态。", refSchema("ScriptLogsResponse")),
					}),
				),
			},
			"/scripts/{name}/schedule": map[string]any{
				"put": openAPIOperation(
					[]string{"scripts"},
					"设置 cron 定时（分 时 日 月 周）",
					[]map[string]any{pathStringParam("name", "脚本文件名。")},
					jsonRequestBody(refSchema("ScriptScheduleRequest")),
					defaulted(map[string]any{
						"200": jsonResponse("含定时信息的脚本状态。", refSchema("ScriptInfo")),
					}),
				),
				"delete": openAPIOperation(
					[]string{"scripts"},
					"取消定时",
					[]map[string]any{pathStringParam("name", "脚本文件名。")},
					nil,
					defaulted(map[string]any{
						"200": jsonResponse("取消结果。", refSchema("ScriptStopResponse")),
					}),
				),
			},
			"/scripts/{name}": map[string]any{
				"delete": openAPIOperation(
					[]string{"scripts"},
					"删除脚本",
					[]map[string]any{pathStringParam("name", "脚本文件名。")},
					nil,
					defaulted(map[string]any{
						"200": jsonResponse("删除结果。", refSchema("ScriptStopResponse")),
					}),
				),
			},
		},
		"components": map[string]any{
			"schemas": map[string]any{
				"APIResponse": objectSchema([]string{"code", "msg", "data"}, map[string]any{
					"code": map[string]any{"type": "integer", "example": 0, "description": "业务状态码，0 表示成功，非 0 表示业务错误。"},
					"msg":  map[string]any{"type": "string", "example": "success", "description": "提示信息，前端可直接用于 Toast 提示。"},
					"data": nullableObjectSchema("实际数据载荷，可以是对象、数组或 null。"),
				}),
				"APIErrorResponse": objectSchema([]string{"code", "msg", "data"}, map[string]any{
					"code": map[string]any{"type": "integer", "example": 400, "description": "非 0 业务错误码。"},
					"msg":  map[string]any{"type": "string", "example": "ref is required"},
					"data": nullableObjectSchema("错误响应当前固定返回 null。"),
				}),
				"HealthResponse": objectSchema([]string{"ok"}, map[string]any{
					"ok": map[string]any{"type": "boolean"},
				}),
				"ReadyResponse": objectSchema([]string{"ready"}, map[string]any{
					"ready": map[string]any{"type": "boolean"},
				}),
				"QRCreateResponse": objectSchema([]string{"session_id", "status", "image_url"}, map[string]any{
					"session_id":   map[string]any{"type": "string"},
					"status":       map[string]any{"type": "string", "example": "pending"},
					"image_url":    map[string]any{"type": "string", "example": "/qr/{session_id}/image"},
					"image_base64": nullableStringSchema("当 as_base64=true 时返回二维码图片 data URI。"),
				}),
				"QRPollResponse": objectSchema([]string{"status"}, map[string]any{
					"status": map[string]any{
						"type": "string",
						"enum": []string{"pending", "scanned", "authorized", "confirmed", "expired", "cancelled", "unknown"},
					},
					"errcode": map[string]any{"type": "integer", "nullable": true},
				}),
				"AccountPublic": objectSchema([]string{"id", "openid", "created_at", "updated_at"}, map[string]any{
					"id":              int64Schema(),
					"openid":          map[string]any{"type": "string"},
					"uin":             nullableInt64Schema(),
					"alias":           nullableStringSchema("账号别名。"),
					"nickname":        nullableStringSchema("账号昵称。"),
					"avatar":          nullableStringSchema("本地头像路径或远程头像 URL。"),
					"status":          nullableStringSchema("账号状态。"),
					"last_checked_at": nullableInt64Schema(),
					"created_at":      int64Schema(),
					"updated_at":      int64Schema(),
				}),
				"RefreshResult": objectSchema([]string{"id", "openid", "status"}, map[string]any{
					"id":       int64Schema(),
					"openid":   map[string]any{"type": "string"},
					"uin":      nullableInt64Schema(),
					"nickname": nullableStringSchema("账号昵称。"),
					"status":   map[string]any{"type": "string", "example": "alive"},
				}),
				"DeleteAccountResponse": objectSchema([]string{"deleted", "openid"}, map[string]any{
					"deleted": int64Schema(),
					"openid":  map[string]any{"type": "string"},
				}),
				"AccountRefRequest": objectSchema(nil, map[string]any{
					"ref": map[string]any{"type": "string", "description": "账号 ID、UIN 或 openid。支持批量操作的接口不传时表示全部账号。"},
				}),
				"RefreshResponse": oneOfSchema(
					refSchema("RefreshResult"),
					arraySchema(refSchema("RefreshResult")),
				),
				"ResyncResponse": oneOfSchema(
					refSchema("AccountPublic"),
					arraySchema(refSchema("AccountPublic")),
				),
				"WxappRequest": objectSchema([]string{"ref", "app_id"}, map[string]any{
					"ref":    map[string]any{"type": "string", "description": "账号 ID、UIN 或 openid。"},
					"app_id": map[string]any{"type": "string"},
				}),
				"OperateWXDataRequest": objectSchema([]string{"ref", "app_id", "payload"}, map[string]any{
					"ref":     map[string]any{"type": "string", "description": "账号 ID、UIN 或 openid。"},
					"app_id":  map[string]any{"type": "string"},
					"payload": freeFormObjectSchema("完整的 operateWxData 请求 JSON。"),
				}),
				"HostSignRequest": objectSchema([]string{"ref", "app_id", "payload"}, map[string]any{
					"ref":     map[string]any{"type": "string", "description": "账号 ID、UIN 或 openid。"},
					"app_id":  map[string]any{"type": "string", "description": "宿主小程序 AppID。"},
					"payload": freeFormObjectSchema("支持 provider+inner_version、plugins 数组或 data 原始 JSON。"),
				}),
				"CallFeatureRequest": objectSchema([]string{"feature", "app_id"}, map[string]any{
					"feature": oneOfSchema(
						map[string]any{"type": "integer", "example": 1004},
						map[string]any{"type": "string", "example": "getHostSign"},
					),
					"app_id":  map[string]any{"type": "string", "description": "小程序 AppID。"},
					"payload": freeFormObjectSchema("功能参数；operateWxData 与 getHostSign 必填。"),
				}),
				"FeatureDefinition": objectSchema([]string{"code", "name", "description", "enabled"}, map[string]any{
					"code":        map[string]any{"type": "integer", "example": 1004},
					"name":        map[string]any{"type": "string", "example": "getHostSign"},
					"description": map[string]any{"type": "string", "example": "verifyplugin HostSign"},
					"enabled":     map[string]any{"type": "boolean"},
				}),
				"ActivitySignRequest": objectSchema([]string{"sign_key"}, map[string]any{
					"sign_key":       map[string]any{"type": "string", "description": "活动配置 signKey。"},
					"token":          map[string]any{"type": "string", "description": "登录态 dt-user-token；生成新 nonce 时参与计算。"},
					"sign_timestamp": map[string]any{"type": "string", "description": "可选固定毫秒时间戳；与 sign_nonce 同时传入时复算固定样本。"},
					"sign_nonce":     map[string]any{"type": "string", "description": "可选固定 nonce；与 sign_timestamp 同时传入。"},
				}),
				"ActivitySignResponse": objectSchema([]string{"signTimestamp", "signNonce", "sign"}, map[string]any{
					"signTimestamp": map[string]any{"type": "string"},
					"signNonce":     map[string]any{"type": "string"},
					"sign":          map[string]any{"type": "string"},
				}),
				"CallFeatureResponse": objectSchema([]string{"feature", "openid", "result"}, map[string]any{
					"feature": map[string]any{"type": "string"},
					"openid":  map[string]any{"type": "string"},
					"result":  freeFormObjectSchema("功能调用结果。"),
				}),
				"WxappResponse": objectSchema([]string{"openid", "result"}, map[string]any{
					"openid": map[string]any{"type": "string"},
					"result": freeFormObjectSchema("wxapp 接口返回结果。"),
				}),
				"ScriptInfo": objectSchema([]string{"name", "size", "updated_at"}, map[string]any{
					"name":        map[string]any{"type": "string", "example": "demo.py"},
					"size":        int64Schema(),
					"updated_at":  int64Schema(),
					"running":     map[string]any{"type": "boolean"},
					"started_at":  nullableInt64Schema(),
					"finished_at": nullableInt64Schema(),
					"exit_code":   map[string]any{"type": "integer", "nullable": true},
					"last_error":  nullableStringSchema("上次运行的错误信息。"),
					"schedule":    nullableStringSchema("cron 表达式。"),
					"next_run_at": nullableInt64Schema(),
				}),
				"ScriptListResponse": objectSchema([]string{"scripts"}, map[string]any{
					"scripts":    arraySchema(refSchema("ScriptInfo")),
					"dir":        map[string]any{"type": "string", "description": "用户脚本目录。"},
					"sdk_dir":    map[string]any{"type": "string", "description": "注入 PYTHONPATH 的 SDK 目录。"},
					"server_url": map[string]any{"type": "string", "description": "注入脚本环境的 YYB_SERVER 地址。"},
					"python":     map[string]any{"type": "string", "nullable": true},
					"python_ok":  map[string]any{"type": "boolean"},
				}),
				"ScriptUploadRequest": objectSchema([]string{"file"}, map[string]any{
					"file": map[string]any{"type": "string", "format": "binary", "description": "*.py 脚本文件。"},
				}),
				"ScriptUploadResponse": objectSchema([]string{"name", "size"}, map[string]any{
					"name": map[string]any{"type": "string"},
					"size": map[string]any{"type": "integer"},
				}),
				"ScriptScheduleRequest": objectSchema([]string{"cron"}, map[string]any{
					"cron": map[string]any{"type": "string", "example": "32 16 * * *", "description": "5 段 cron：分 时 日 月 周。"},
				}),
				"ScriptRunRequest": objectSchema(nil, map[string]any{
					"args": map[string]any{"type": "string", "description": "传给脚本的命令行参数（shell 风格，支持引号与转义），同时注入环境变量 YYB_SCRIPT_ARGS。"},
				}),
				"ScriptLogsResponse": objectSchema([]string{"name", "content", "running"}, map[string]any{
					"name":        map[string]any{"type": "string"},
					"content":     map[string]any{"type": "string", "description": "最多 256 KiB 的日志尾部。"},
					"running":     map[string]any{"type": "boolean"},
					"started_at":  nullableInt64Schema(),
					"finished_at": nullableInt64Schema(),
					"exit_code":   map[string]any{"type": "integer", "nullable": true},
					"last_error":  nullableStringSchema("上次运行的错误信息。"),
				}),
				"ScriptStopResponse": objectSchema(nil, map[string]any{
					"stopped":     map[string]any{"type": "string", "nullable": true},
					"deleted":     map[string]any{"type": "string", "nullable": true},
					"unscheduled": map[string]any{"type": "string", "nullable": true},
				}),
			},
		},
	}
}

func openAPIOperation(tags []string, summary string, parameters []map[string]any, requestBody map[string]any, responses map[string]any) map[string]any {
	out := map[string]any{
		"tags":      tags,
		"summary":   summary,
		"responses": responses,
	}
	if len(parameters) > 0 {
		out["parameters"] = parameters
	}
	if requestBody != nil {
		out["requestBody"] = requestBody
	}
	return out
}

func defaulted(responses map[string]any) map[string]any {
	responses["default"] = jsonErrorResponse("错误响应。")
	return responses
}

func jsonResponse(description string, schema map[string]any) map[string]any {
	return map[string]any{
		"description": description,
		"content": map[string]any{
			"application/json": map[string]any{
				"schema": apiResponseSchema(schema),
			},
		},
	}
}

func jsonErrorResponse(description string) map[string]any {
	return map[string]any{
		"description": description,
		"content": map[string]any{
			"application/json": map[string]any{
				"schema": refSchema("APIErrorResponse"),
			},
		},
	}
}

func imageResponse(description string) map[string]any {
	return map[string]any{
		"description": description,
		"content": map[string]any{
			"image/jpeg": map[string]any{
				"schema": map[string]any{"type": "string", "format": "binary"},
			},
		},
	}
}

func jsonRequestBody(schema map[string]any) map[string]any {
	return map[string]any{
		"required": true,
		"content": map[string]any{
			"application/json": map[string]any{
				"schema": schema,
			},
		},
	}
}

func jsonOptionalRequestBody(schema map[string]any) map[string]any {
	return map[string]any{
		"required": false,
		"content": map[string]any{
			"application/json": map[string]any{
				"schema": schema,
			},
		},
	}
}

func pathStringParam(name, description string) map[string]any {
	return map[string]any{
		"name":        name,
		"in":          "path",
		"description": description,
		"required":    true,
		"schema":      map[string]any{"type": "string"},
	}
}

func queryStringParam(name, description string, required bool) map[string]any {
	return map[string]any{
		"name":        name,
		"in":          "query",
		"description": description,
		"required":    required,
		"schema":      map[string]any{"type": "string"},
	}
}

func boolQueryParam(name, description string) map[string]any {
	return map[string]any{
		"name":        name,
		"in":          "query",
		"description": description,
		"required":    false,
		"schema":      map[string]any{"type": "boolean"},
	}
}

func oneOfSchema(schemas ...map[string]any) map[string]any {
	return map[string]any{"oneOf": schemas}
}

func refSchema(name string) map[string]any {
	return map[string]any{"$ref": "#/components/schemas/" + name}
}

func arraySchema(item map[string]any) map[string]any {
	return map[string]any{
		"type":  "array",
		"items": item,
	}
}

func objectSchema(required []string, properties map[string]any) map[string]any {
	schema := map[string]any{
		"type":       "object",
		"properties": properties,
	}
	if len(required) > 0 {
		schema["required"] = required
	}
	return schema
}

func apiResponseSchema(dataSchema map[string]any) map[string]any {
	if dataSchema == nil {
		dataSchema = nullableObjectSchema("实际数据载荷。")
	}
	return objectSchema([]string{"code", "msg", "data"}, map[string]any{
		"code": map[string]any{"type": "integer", "example": 0, "description": "业务状态码，0 表示成功，非 0 表示业务错误。"},
		"msg":  map[string]any{"type": "string", "example": "success", "description": "提示信息，前端可直接用于 Toast 提示。"},
		"data": dataSchema,
	})
}

func freeFormObjectSchema(description string) map[string]any {
	return map[string]any{
		"type":                 "object",
		"description":          description,
		"additionalProperties": true,
		"nullable":             true,
	}
}

func nullableObjectSchema(description string) map[string]any {
	return map[string]any{
		"type":                 "object",
		"description":          description,
		"additionalProperties": true,
		"nullable":             true,
	}
}

func nullableStringSchema(description string) map[string]any {
	return map[string]any{
		"type":        "string",
		"description": description,
		"nullable":    true,
	}
}

func int64Schema() map[string]any {
	return map[string]any{"type": "integer", "format": "int64"}
}

func nullableInt64Schema() map[string]any {
	return map[string]any{"type": "integer", "format": "int64", "nullable": true}
}
