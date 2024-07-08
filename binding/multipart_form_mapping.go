// Copyright 2019 Gin Core Team. All rights reserved.
// Use of this source code is governed by a MIT style
// license that can be found in the LICENSE file.

package binding

import (
	"errors"
	"mime/multipart"
	"net/http"
	"reflect"
)

type multipartRequest http.Request

// 接口实现校验
var _ setter = (*multipartRequest)(nil)

var (
	// 错误的multipart.FileHeader
	ErrMultiFileHeader = errors.New("unsupported field type for multipart.FileHeader")

	// []*multipart.FileHeader的长度错误
	ErrMultiFileHeaderLenInvalid = errors.New("unsupported len of array for []*multipart.FileHeader")
)

// 尝试绑定form file到value中
func (r *multipartRequest) TrySet(value reflect.Value, field reflect.StructField, key string, opt setOptions) (bool, error) {
	// 有file使用setByMultipartFormFile绑定file的值
	if files := r.MultipartForm.File[key]; len(files) != 0 {
		return setByMultipartFormFile(value, field, files)
	}

	// 没有file通过setByForm进行值绑定
	return setByForm(value, field, r.MultipartForm.Value, key, opt)
}

// 设置MultipartForm中的file值
func setByMultipartFormFile(value reflect.Value, field reflect.StructField, files []*multipart.FileHeader) (isSet bool, err error) {
	switch value.Kind() {
	case reflect.Ptr:
		// 如果值为*multipart.FileHeader，通过反射设置值
		switch value.Interface().(type) {
		case *multipart.FileHeader:
			// 默认设置第0位的值
			value.Set(reflect.ValueOf(files[0]))
			return true, nil
		}
	case reflect.Struct:
		// 如果值为multipart.FileHeader，通过反射设置值的指针值
		switch value.Interface().(type) {
		case multipart.FileHeader:
			// 默认设置第0位的值
			value.Set(reflect.ValueOf(*files[0]))
			return true, nil
		}
	case reflect.Slice:
		// 通过setArrayOfMultipartFormFiles循环调用setByMultipartFormFile进行file的值设置
		slice := reflect.MakeSlice(value.Type(), len(files), len(files))
		isSet, err = setArrayOfMultipartFormFiles(slice, field, files)
		if err != nil || !isSet {
			return isSet, err
		}
		// 将切片的值赋值给value
		value.Set(slice)
		return true, nil
	case reflect.Array:
		// 和Slice逻辑一样
		return setArrayOfMultipartFormFiles(value, field, files)
	}
	// 绑定值不匹配
	return false, ErrMultiFileHeader
}

// 设置多file的MultipartForm
func setArrayOfMultipartFormFiles(value reflect.Value, field reflect.StructField, files []*multipart.FileHeader) (isSet bool, err error) {
	// 需要绑定的值长度和multipart.FileHeader长度不匹配
	if value.Len() != len(files) {
		return false, ErrMultiFileHeaderLenInvalid
	}
	// 逐位设置file的值
	for i := range files {
		set, err := setByMultipartFormFile(value.Index(i), field, files[i:i+1])
		if err != nil || !set {
			return set, err
		}
	}
	return true, nil
}
