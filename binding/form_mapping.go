// Copyright 2014 Manu Martinez-Almeida. All rights reserved.
// Use of this source code is governed by a MIT style
// license that can be found in the LICENSE file.

package binding

import (
	"errors"
	"fmt"
	"reflect"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin/internal/bytesconv"
	"github.com/gin-gonic/gin/internal/json"
)

var (
	// 未知类型
	errUnknownType = errors.New("unknown type")

	// 转换为map[string][]string错误
	ErrConvertMapStringSlice = errors.New("can not convert to map slices of strings")

	// 转换为map[string]string错误
	ErrConvertToMapString = errors.New("can not convert to map of strings")
)

// 映射uri的值
func mapURI(ptr any, m map[string][]string) error {
	return mapFormByTag(ptr, m, "uri")
}

// 映射form的值
func mapForm(ptr any, form map[string][]string) error {
	return mapFormByTag(ptr, form, "form")
}

// 通过tag映射form的值
func MapFormWithTag(ptr any, form map[string][]string, tag string) error {
	return mapFormByTag(ptr, form, tag)
}

// 空的field
var emptyField = reflect.StructField{}

func mapFormByTag(ptr any, form map[string][]string, tag string) error {
	// 反射获取ptr的值
	ptrVal := reflect.ValueOf(ptr)
	var pointed any
	// ptr的类型为reflect.Ptr
	if ptrVal.Kind() == reflect.Ptr {
		ptrVal = ptrVal.Elem()
		pointed = ptrVal.Interface()
	}
	// ptr的类型为reflect.Map || ptrVal.Type().Key()的类型为reflect.String
	if ptrVal.Kind() == reflect.Map &&
		ptrVal.Type().Key().Kind() == reflect.String {
		if pointed != nil {
			ptr = pointed
		}
		// 当类型为map[string][]string || map[string]string，进行ptr赋值
		return setFormMap(ptr, form)
	}

	// form强转为formSource（map[string][]string），进行赋值处理
	return mappingByPtr(ptr, formSource(form), tag)
}

// 在遍历struct时尝试进行赋值
type setter interface {
	TrySet(value reflect.Value, field reflect.StructField, key string, opt setOptions) (isSet bool, err error)
}

type formSource map[string][]string

// 接口实现校验
var _ setter = (nil)

// 尝试用request's form给formSource设置值
func (form formSource) TrySet(value reflect.Value, field reflect.StructField, tagValue string, opt setOptions) (isSet bool, err error) {
	return setByForm(value, field, form, tagValue, opt)
}

// 通过ptr绑定值
func mappingByPtr(ptr any, setter setter, tag string) error {
	_, err := mapping(reflect.ValueOf(ptr), emptyField, setter, tag)
	return err
}

// 通过不同类型绑定值的方法
func mapping(value reflect.Value, field reflect.StructField, setter setter, tag string) (bool, error) {
	// 忽略-的tag类型
	if field.Tag.Get(tag) == "-" {
		return false, nil
	}

	// value的反射类型
	vKind := value.Kind()

	// 反射类型为reflect.Ptr
	if vKind == reflect.Ptr {
		var isNew bool
		vPtr := value
		// 判断value是否是空的指针类型
		if value.IsNil() {
			// 如果是value空的指针类型，则通过反射创建新的vPtr
			isNew = true
			vPtr = reflect.New(value.Type().Elem())
		}
		// TODO：
		isSet, err := mapping(vPtr.Elem(), field, setter, tag)
		if err != nil {
			return false, err
		}
		if isNew && isSet {
			value.Set(vPtr)
		}
		return isSet, nil
	}

	// 反射类型不为reflect.Struct || 或者匿名字段
	if vKind != reflect.Struct || !field.Anonymous {
		// 尝试通过tag进行设置
		ok, err := tryToSetValue(value, field, setter, tag)
		if err != nil {
			return false, err
		}
		if ok {
			return true, nil
		}
	}

	// 反射类型为reflect.Struct
	if vKind == reflect.Struct {
		// 获取反射字段类型
		tValue := value.Type()

		var isSet bool
		// 每个字段进行设置值
		for i := 0; i < value.NumField(); i++ {
			sf := tValue.Field(i)
			if sf.PkgPath != "" && !sf.Anonymous { // unexported
				continue
			}
			// 每个字段递归设置字段值
			ok, err := mapping(value.Field(i), sf, setter, tag)
			if err != nil {
				return false, err
			}
			// 只有要字段设置过就返回true
			isSet = isSet || ok
		}
		return isSet, nil
	}
	// 类型不匹配返回false
	return false, nil
}

// TODO
type setOptions struct {
	isDefaultExists bool
	defaultValue    string
}

// 尝试设置值，非强制，一般不会报错
func tryToSetValue(value reflect.Value, field reflect.StructField, setter setter, tag string) (bool, error) {
	var tagValue string
	var setOpt setOptions

	tagValue = field.Tag.Get(tag)
	tagValue, opts := head(tagValue, ",")

	// 如果tagValue为空，默认值为FieldName
	if tagValue == "" {
		tagValue = field.Name
	}
	// 如果如果tagValue为空还是为空，则field为emptyField，返回false
	if tagValue == "" {
		return false, nil
	}

	var opt string
	// 将opts中的,全部找出来进行分割
	for len(opts) > 0 {
		opt, opts = head(opts, ",")

		// 每份前面的字符串判断是否可以用=分割，取k=default的字符串
		if k, v := head(opt, "="); k == "default" {
			// 设置isDefaultExists
			setOpt.isDefaultExists = true
			//　设置defaultValue
			setOpt.defaultValue = v
		}
	}

	return setter.TrySet(value, field, tagValue, setOpt)
}

// 通过form设置值
func setByForm(value reflect.Value, field reflect.StructField, form map[string][]string, tagValue string, opt setOptions) (isSet bool, err error) {
	// 获取tag值
	vs, ok := form[tagValue]
	if !ok && !opt.isDefaultExists {
		return false, nil
	}

	switch value.Kind() {
	case reflect.Slice:
		// 获取不到tagValue
		if !ok {
			// 设置opt.defaultValue默认值
			vs = []string{opt.defaultValue}
		}
		// 通过对应类型设置Slice的值
		return true, setSlice(vs, value, field)
	case reflect.Array:
		if !ok {
			vs = []string{opt.defaultValue}
		}
		if len(vs) != value.Len() {
			return false, fmt.Errorf("%q is not valid value for %s", vs, value.Type().String())
		}
		// 通过对应类型设置Array的值
		return true, setArray(vs, value, field)
	default:
		// 默认通过value的反射类型设置值
		var val string
		if !ok {
			val = opt.defaultValue
		}

		if len(vs) > 0 {
			val = vs[0]
		}
		return true, setWithProperType(val, value, field)
	}
}

// 通过value的不同反射类型设置值，内部原理一样，若有值则设置，没值设置默认值
func setWithProperType(val string, value reflect.Value, field reflect.StructField) error {
	switch value.Kind() {
	case reflect.Int:
		return setIntField(val, 0, value)
	case reflect.Int8:
		return setIntField(val, 8, value)
	case reflect.Int16:
		return setIntField(val, 16, value)
	case reflect.Int32:
		return setIntField(val, 32, value)
	case reflect.Int64:
		switch value.Interface().(type) {
		case time.Duration:
			return setTimeDuration(val, value)
		}
		return setIntField(val, 64, value)
	case reflect.Uint:
		return setUintField(val, 0, value)
	case reflect.Uint8:
		return setUintField(val, 8, value)
	case reflect.Uint16:
		return setUintField(val, 16, value)
	case reflect.Uint32:
		return setUintField(val, 32, value)
	case reflect.Uint64:
		return setUintField(val, 64, value)
	case reflect.Bool:
		return setBoolField(val, value)
	case reflect.Float32:
		return setFloatField(val, 32, value)
	case reflect.Float64:
		return setFloatField(val, 64, value)
	case reflect.String:
		value.SetString(val)
	case reflect.Struct:
		switch value.Interface().(type) {
		case time.Time:
			return setTimeField(val, field, value)
		}
		return json.Unmarshal(bytesconv.StringToBytes(val), value.Addr().Interface())
	case reflect.Map:
		return json.Unmarshal(bytesconv.StringToBytes(val), value.Addr().Interface())
	default:
		return errUnknownType
	}
	return nil
}

// 设置int类型的值，如果val为空，设置为0，否则设置为val的int类型值
func setIntField(val string, bitSize int, field reflect.Value) error {
	if val == "" {
		val = "0"
	}
	intVal, err := strconv.ParseInt(val, 10, bitSize)
	if err == nil {
		field.SetInt(intVal)
	}
	return err
}

// 设置uint类型的值，如果val为空，设置为0，否则设置为val的uint类型值
func setUintField(val string, bitSize int, field reflect.Value) error {
	if val == "" {
		val = "0"
	}
	uintVal, err := strconv.ParseUint(val, 10, bitSize)
	if err == nil {
		field.SetUint(uintVal)
	}
	return err
}

// 设置bool类型的值，如果val为空，设置为false，否则设置为val的bool类型值
func setBoolField(val string, field reflect.Value) error {
	if val == "" {
		val = "false"
	}
	boolVal, err := strconv.ParseBool(val)
	if err == nil {
		field.SetBool(boolVal)
	}
	return err
}

// 设置float类型的值，如果val为空，设置为0.0，否则设置为val的float类型值
func setFloatField(val string, bitSize int, field reflect.Value) error {
	if val == "" {
		val = "0.0"
	}
	floatVal, err := strconv.ParseFloat(val, bitSize)
	if err == nil {
		field.SetFloat(floatVal)
	}
	return err
}

func setTimeField(val string, structField reflect.StructField, value reflect.Value) error {
	// 找到默认的timeFormat格式，没有设置则为time.RFC3339格式（"2006-01-02T15:04:05Z07:00"）
	timeFormat := structField.Tag.Get("time_format")
	if timeFormat == "" {
		timeFormat = time.RFC3339
	}

	switch tf := strings.ToLower(timeFormat); tf {
	case "unix", "unixnano": // 转为unix
		tv, err := strconv.ParseInt(val, 10, 64)
		if err != nil {
			return err
		}

		d := time.Duration(1)
		if tf == "unixnano" {
			d = time.Second
		}

		t := time.Unix(tv/int64(d), tv%int64(d))
		value.Set(reflect.ValueOf(t))
		return nil
	}

	if val == "" {
		value.Set(reflect.ValueOf(time.Time{}))
		return nil
	}

	l := time.Local
	// 判断time_utc的值
	if isUTC, _ := strconv.ParseBool(structField.Tag.Get("time_utc")); isUTC {
		l = time.UTC
	}

	if locTag := structField.Tag.Get("time_location"); locTag != "" {
		loc, err := time.LoadLocation(locTag)
		if err != nil {
			return err
		}
		l = loc
	}

	// 转换为对应时区的时间值
	t, err := time.ParseInLocation(timeFormat, val, l)
	if err != nil {
		return err
	}

	value.Set(reflect.ValueOf(t))
	return nil
}

// 通过value传进来的reflect类型，设置Array
func setArray(vals []string, value reflect.Value, field reflect.StructField) error {
	for i, s := range vals {
		// 逐个设置属性值
		err := setWithProperType(s, value.Index(i), field)
		if err != nil {
			return err
		}
	}
	return nil
}

// 设置Slice通过setArray实现
func setSlice(vals []string, value reflect.Value, field reflect.StructField) error {
	slice := reflect.MakeSlice(value.Type(), len(vals), len(vals))
	err := setArray(vals, slice, field)
	if err != nil {
		return err
	}
	value.Set(slice)
	return nil
}

// 设置TimeDuration类型
func setTimeDuration(val string, value reflect.Value) error {
	d, err := time.ParseDuration(val)
	if err != nil {
		return err
	}
	value.Set(reflect.ValueOf(d))
	return nil
}

func head(str, sep string) (head string, tail string) {
	// sep在str中的位置
	idx := strings.Index(str, sep)
	// 不存在直接返回
	if idx < 0 {
		return str, ""
	}
	// 通过idx将str分为head和tail
	return str[:idx], str[idx+len(sep):]
}

// 通过formMap设置ptr值
func setFormMap(ptr any, form map[string][]string) error {
	// 反射获取ptr的elem
	el := reflect.TypeOf(ptr).Elem()

	// 判断el的类型，这个分支为map[string][]string
	if el.Kind() == reflect.Slice {
		// 确保ptr的类型为map[string][]string，为后面循环赋值做前置准备
		ptrMap, ok := ptr.(map[string][]string)
		if !ok {
			return ErrConvertMapStringSlice
		}
		// 遍历赋值
		for k, v := range form {
			ptrMap[k] = v
		}

		return nil
	}

	// 判断el的类型，这个分支为map[string]string
	ptrMap, ok := ptr.(map[string]string)
	if !ok {
		return ErrConvertToMapString
	}
	// 确保ptr的类型为map[string]string，为后面循环赋值做前置准备
	for k, v := range form {
		// TODO：？？ 从尾部开始插入
		ptrMap[k] = v[len(v)-1]
	}

	return nil
}
