// Copyright 2014 Manu Martinez-Almeida. All rights reserved.
// Use of this source code is governed by a MIT style
// license that can be found in the LICENSE file.

package render

import (
	"fmt"
	"net/http"
)

// Redirect 结构体
type Redirect struct {
	// http code码
	Code int
	// 源http.Request地址
	Request *http.Request
	// 目的地址（url）
	Location string
}

// Render 重定向http request到location地址
func (r Redirect) Render(w http.ResponseWriter) error {
	// 判断重定向的http code
	if (r.Code < http.StatusMultipleChoices || r.Code > http.StatusPermanentRedirect) && r.Code != http.StatusCreated {
		panic(fmt.Sprintf("Cannot redirect with status code %d", r.Code))
	}
	// 将writer、reader、code和location进行重定向
	http.Redirect(w, r.Request, r.Location, r.Code)
	return nil
}

// 重定向不需要写入ContentType
func (r Redirect) WriteContentType(http.ResponseWriter) {}
