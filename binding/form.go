// Copyright 2014 Manu Martinez-Almeida. All rights reserved.
// Use of this source code is governed by a MIT style
// license that can be found in the LICENSE file.

package binding

import (
	"errors"
	"net/http"
)

const defaultMemory = 32 << 20

type formBinding struct{}
type formPostBinding struct{}
type formMultipartBinding struct{}

func (formBinding) Name() string {
	return "form"
}

// 绑定form的值
func (formBinding) Bind(req *http.Request, obj any) error {
	// 解析form表单
	if err := req.ParseForm(); err != nil {
		return err
	}
	// 解析multipart form表单
	if err := req.ParseMultipartForm(defaultMemory); err != nil && !errors.Is(err, http.ErrNotMultipart) {
		return err
	}
	// 绑定form值
	if err := mapForm(obj, req.Form); err != nil {
		return err
	}
	// 校验obj
	return validate(obj)
}

func (formPostBinding) Name() string {
	return "form-urlencoded"
}

func (formPostBinding) Bind(req *http.Request, obj any) error {
	// 解析form表单
	if err := req.ParseForm(); err != nil {
		return err
	}
	// 绑定form值
	if err := mapForm(obj, req.PostForm); err != nil {
		return err
	}
	// 校验obj
	return validate(obj)
}

func (formMultipartBinding) Name() string {
	return "multipart/form-data"
}

func (formMultipartBinding) Bind(req *http.Request, obj any) error {
	// 解析multipart form表单
	if err := req.ParseMultipartForm(defaultMemory); err != nil {
		return err
	}
	// 通过ptr绑定值
	if err := mappingByPtr(obj, (*multipartRequest)(req), "form"); err != nil {
		return err
	}
	// 校验obj
	return validate(obj)
}
