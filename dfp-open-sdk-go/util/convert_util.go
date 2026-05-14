package util

import (
	"encoding/json"
	"fmt"
	"reflect"
	"strconv"
)

func ConvertJsonToForm(jsonStr string) map[string]string {
	var source map[string]interface{}
	var result = make(map[string]string)
	// 使用json.Unmarshal将JSON字符串解析到map中
	err := json.Unmarshal([]byte(jsonStr), &source)

	if err != nil {
		return nil
	}

	ConvertRecursive(source, &result, "")

	return result
}

// ConvertStructToForm 将结构体转换为适用于开放银行表单格式的map格式
// object 结构体
func ConvertStructToForm(object interface{}) map[string]string {
	marshal, _ := json.Marshal(object)
	result := ConvertJsonToForm(string(marshal))
	return result
}

func structToMap(i interface{}) map[string]interface{} {
	m := make(map[string]interface{})
	v := reflect.ValueOf(i)

	// 使用reflect.ValueOf获取struct的值
	switch v.Kind() {
	case reflect.Ptr:
		v = v.Elem()
		fallthrough
	case reflect.Struct:
		for i := 0; i < v.NumField(); i++ {
			field := v.Field(i)
			key := v.Type().Field(i).Name
			// 如果字段有map_key标签，则使用标签的值作为map的键
			if tag := v.Type().Field(i).Tag.Get("map_key"); tag != "" {
				key = tag
			}
			m[key] = field.Interface()
		}
	}

	return m
}

func ConvertRecursive(source interface{}, result *map[string]string, prefix string) {
	switch sourceValue := source.(type) {
	case map[string]interface{}:
		for key, value := range sourceValue {
			switch v := value.(type) {
			case []interface{}:
				for index, v := range v {
					var newPrefix = prefix + key + "[" + strconv.Itoa(index) + "]"
					if prefix != "" {
						newPrefix = prefix + ". " + key + "[" + strconv.Itoa(index) + "]"
					}
					ConvertRecursive(v, result, newPrefix)
				}
				break
			default:
				var newPrefix = key
				if prefix != "" {
					newPrefix = prefix + "." + key
				}
				ConvertRecursive(value, result, newPrefix)
			}
		}
		break
	case []interface{}:
		for index, value := range sourceValue {
			switch v := value.(type) {
			case []interface{}:
				for i, data := range v {
					ConvertRecursive(data, result, prefix+strconv.Itoa(index)+"["+strconv.Itoa(i)+"]")
				}
				break
			default:
				ConvertRecursive(value, result, strconv.Itoa(index))
			}
		}
		break
	default:
		if source != nil {
			(*result)[prefix] = fmt.Sprintf("%v", source)
		}
		break
	}
}
