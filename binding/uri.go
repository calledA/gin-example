// Copyright 2018 Gin Core Team. All rights reserved.
// Use of this source code is governed by a MIT style
// license that can be found in the LICENSE file.

package binding

type uriBinding struct{}

func (uriBinding) Name() string {
	return "uri"
}

// 绑定URI的值
func (uriBinding) BindUri(m map[string][]string, obj any) error {
	// 映射uri的字段值
	if err := mapURI(obj, m); err != nil {
		return err
	}
	// 绑定值之后校验值
	return validate(obj)
}
