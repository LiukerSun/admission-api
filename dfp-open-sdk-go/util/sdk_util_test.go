package util

import "testing"

func TestEmptyString(t *testing.T) {
	if EmptyString("") {
		t.Log("kong")
	}

	if EmptyString("  ") {
		t.Log("kong")
	}

	if EmptyString("1") {
		t.Log("kong")
	}
}

func TestJointMap(t *testing.T) {
	//var mapInfo map[string]string{
	//	"a": "b"
	//}
}
