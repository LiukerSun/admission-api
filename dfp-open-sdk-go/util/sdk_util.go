package util

import (
	"net/url"
	"strings"
)

func CheckReqUri(reqUri string) string {
	if reqUri == "" {
		return reqUri
	}

	if !strings.HasPrefix(reqUri, "/") {
		reqUri = "/" + reqUri
	}

	if strings.HasSuffix(reqUri, "/") {
		reqUri = reqUri[:len(reqUri)-1]
	}

	return reqUri
}

func JointMap(paramsMap map[string]string) string {
	values := url.Values{}

	for k, v := range paramsMap {
		values.Add(k, v)
	}

	encodeString := values.Encode()
	return encodeString
}

func EmptyString(str string) bool {
	return len(strings.TrimSpace(str)) == 0
}
