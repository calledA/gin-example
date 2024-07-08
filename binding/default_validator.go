// Copyright 2017 Manu Martinez-Almeida. All rights reserved.
// Use of this source code is governed by a MIT style
// license that can be found in the LICENSE file.

package binding

import (
	"fmt"
	"reflect"
	"strings"
	"sync"

	"github.com/go-playground/validator/v10"
)

// 默认的validator，实现了StructValidator接口
type defaultValidator struct {
	once     sync.Once
	validate *validator.Validate
}

// validator的错误Slice
type SliceValidationError []error

// 将SliceValidationError中所有错误通过\n连接成一个字符串
func (err SliceValidationError) Error() string {
	n := len(err)
	switch n {
	case 0:
		return ""
	default:
		var b strings.Builder
		// 如果只有一条，就不用追加\n
		if err[0] != nil {
			fmt.Fprintf(&b, "[%d]: %s", 0, err[0].Error())
		}
		// 超过一条，通过\n连接
		if n > 1 {
			for i := 1; i < n; i++ {
				if err[i] != nil {
					b.WriteString("\n")
					fmt.Fprintf(&b, "[%d]: %s", i, err[i].Error())
				}
			}
		}
		return b.String()
	}
}

// 接口实现校验
var _ StructValidator = (*defaultValidator)(nil)

// ValidateStruct接受any类型，但是只会处理结构体、指针和指向指针的类型（Slice和Array）
func (v *defaultValidator) ValidateStruct(obj any) error {
	if obj == nil {
		return nil
	}

	// 反射获取obj的值
	value := reflect.ValueOf(obj)
	// 反射获取value的类型
	switch value.Kind() {
	case reflect.Ptr:
		// 递归校验Ptr的值
		return v.ValidateStruct(value.Elem().Interface())
	case reflect.Struct:
		return v.validateStruct(obj)
	case reflect.Slice, reflect.Array:
		count := value.Len()
		// 类型为Slice和Array，创建等长的SliceValidationError记录校验错误
		validateRet := make(SliceValidationError, 0)
		for i := 0; i < count; i++ {
			// 递归校验对应index的值
			if err := v.ValidateStruct(value.Index(i).Interface()); err != nil {
				validateRet = append(validateRet, err)
			}
		}
		if len(validateRet) == 0 {
			return nil
		}
		return validateRet
	default:
		return nil
	}
}

// validateStruct校验struct类型
func (v *defaultValidator) validateStruct(obj any) error {
	// 获取v.validate单例
	v.lazyinit()
	// 使用validate校验struct类型
	return v.validate.Struct(obj)
}

// 返货默认的validator engine
func (v *defaultValidator) Engine() any {
	v.lazyinit()
	return v.validate
}

func (v *defaultValidator) lazyinit() {
	// 单例模式，单例创建validator
	v.once.Do(func() {
		v.validate = validator.New()
		v.validate.SetTagName("binding")
	})
}
