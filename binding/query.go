// Copyright 2017 Manu Martinez-Almeida. All rights reserved.
// Use of this source code is governed by a MIT style
// license that can be found in the LICENSE file.

package binding

import "net/http"

type queryBinding struct{}

func (queryBinding) Name() string {
	return "query"
}

// 通过req.URL.Query()的参数进行值绑定
func (queryBinding) Bind(req *http.Request, obj any) error {
	// 获取Query参数
	values := req.URL.Query()
	// 绑定form值
	if err := mapForm(obj, values); err != nil {
		return err
	}
	// 绑定值之后，通过Validator校验参数
	return validate(obj)
}
