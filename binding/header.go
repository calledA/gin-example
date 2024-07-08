// Copyright 2022 Gin Core Team. All rights reserved.
// Use of this source code is governed by a MIT style
// license that can be found in the LICENSE file.

package binding

import (
	"net/http"
	"net/textproto"
	"reflect"
)

type headerBinding struct{}

func (headerBinding) Name() string {
	return "header"
}

// 通过req.Header绑定值
func (headerBinding) Bind(req *http.Request, obj any) error {
	if err := mapHeader(obj, req.Header); err != nil {
		return err
	}
	// 绑定值之后校验值
	return validate(obj)
}

func mapHeader(ptr any, h map[string][]string) error {
	return mappingByPtr(ptr, headerSource(h), "header")
}

type headerSource map[string][]string

// 校验headerSource结构体是否实现了setter接口
var _ setter = headerSource(nil)

// 通过setByForm设置value的值
func (hs headerSource) TrySet(value reflect.Value, field reflect.StructField, tagValue string, opt setOptions) (bool, error) {
	return setByForm(value, field, hs, textproto.CanonicalMIMEHeaderKey(tagValue), opt)
}
