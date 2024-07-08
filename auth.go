// Copyright 2014 Manu Martinez-Almeida. All rights reserved.
// Use of this source code is governed by a MIT style
// license that can be found in the LICENSE file.

package gin

import (
	"crypto/subtle"
	"encoding/base64"
	"github.com/gin-gonic/gin/internal/bytesconv"
	"net/http"
	"strconv"
)

// 用户auth名称
const AuthUserKey = "user"

// 授权登录的use/pwd键值对
type Accounts map[string]string

// 授权对
type authPair struct {
	value string
	user  string
}

type authPairs []authPair

func (a authPairs) searchCredential(authValue string) (string, bool) {
	if authValue == "" {
		return "", false
	}
	// 检索authValue是否存在
	for _, pair := range a {
		if subtle.ConstantTimeCompare(bytesconv.StringToBytes(pair.value), bytesconv.StringToBytes(authValue)) == 1 {
			return pair.user, true
		}
	}
	return "", false
}

// 基础的HTTP Authorization中间件，accounts是一个key为user，value为password的map,realm为Basic realm的值
func BasicAuthForRealm(accounts Accounts, realm string) HandlerFunc {
	// 默认为Authorization Required
	if realm == "" {
		realm = "Authorization Required"
	}
	realm = "Basic realm=" + strconv.Quote(realm)
	// 处理为authPairs类型
	pairs := processAccounts(accounts)
	return func(c *Context) {
		// 查找request中的Authorization header
		user, found := pairs.searchCredential(c.requestHeader("Authorization"))
		if !found {
			// 未找到Authorization header，返回401，并且中断请求
			c.Header("WWW-Authenticate", realm)
			c.AbortWithStatus(http.StatusUnauthorized)
			return
		}

		// 找到Authorization header，将Authorization放到context中，key为AuthUserKey，方便后续使用
		c.Set(AuthUserKey, user)
	}
}

// 返回基础的HTTP Authorization中间件，携带map[string]string的参数，key为user，value为password
func BasicAuth(accounts Accounts) HandlerFunc {
	return BasicAuthForRealm(accounts, "")
}

// 将Accounts中的map转换为authPairs类型
func processAccounts(accounts Accounts) authPairs {
	// 校验是否为空
	length := len(accounts)
	assert1(length > 0, "Empty list of authorized credentials")
	pairs := make(authPairs, 0, length)
	// 转换Accounts
	for user, password := range accounts {
		assert1(user != "", "User can not be empty")
		// 使用authorizationHeader生成user和password的value
		value := authorizationHeader(user, password)
		pairs = append(pairs, authPair{
			value: value,
			user:  user,
		})
	}
	return pairs
}

// 将user和password进行base64 encoding
func authorizationHeader(user, password string) string {
	base := user + ":" + password
	return "Basic " + base64.StdEncoding.EncodeToString(bytesconv.StringToBytes(base))
}
