//go:build darwin && cgo

package systemproxy

/*
#cgo LDFLAGS: -framework Foundation -framework Security -framework SystemConfiguration
#include <stdlib.h>
char *peekly_read_proxy(const char *service, char **message);
char *peekly_replace_proxy(const char *service, const char *expected, const char *desired, int interactive);
*/
import "C"

import (
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"unsafe"
)

type darwinDriver struct{}

func newDriver() driver { return darwinDriver{} }

func readProxy(service string) (string, fields, error) {
	input := C.CString(service)
	defer C.free(unsafe.Pointer(input))
	var message *C.char
	output := C.peekly_read_proxy(input, &message)
	if output == nil {
		return "", nil, macError(message)
	}
	defer C.free(unsafe.Pointer(output))
	var result struct {
		Service  string `json:"service"`
		Settings fields `json:"settings"`
	}
	err := json.Unmarshal([]byte(C.GoString(output)), &result)
	return result.Service, result.Settings, err
}

func (darwinDriver) prepare(host string, port int) (record, error) {
	service, before, err := readProxy("")
	if err != nil {
		return record{}, err
	}
	return record{Service: service, Before: before, After: fields{
		"HTTPEnable": 1, "HTTPProxy": host, "HTTPPort": port,
		"HTTPSEnable": 1, "HTTPSProxy": host, "HTTPSPort": port,
		"ProxyAutoConfigEnable": 0, "ProxyAutoDiscoveryEnable": 0,
	}}, nil
}

func (darwinDriver) current(service string) (fields, error) {
	_, state, err := readProxy(service)
	return state, err
}

func (darwinDriver) replace(service string, expected, next fields, interactive bool) error {
	old, err := json.Marshal(expected)
	if err != nil {
		return err
	}
	data, err := json.Marshal(next)
	if err != nil {
		return err
	}
	svc, before, after := C.CString(service), C.CString(string(old)), C.CString(string(data))
	defer C.free(unsafe.Pointer(svc))
	defer C.free(unsafe.Pointer(before))
	defer C.free(unsafe.Pointer(after))
	var prompt C.int
	if interactive {
		prompt = 1
	}
	if message := C.peekly_replace_proxy(svc, before, after, prompt); message != nil {
		return macError(message)
	}
	return nil
}

func (darwinDriver) references(state fields, endpoint string) bool {
	for _, scheme := range []string{"HTTP", "HTTPS"} {
		host, _ := state[scheme+"Proxy"].(string)
		port := fmt.Sprint(state[scheme+"Port"])
		enabled := fmt.Sprint(state[scheme+"Enable"])
		if (enabled == "1" || enabled == "true") && net.JoinHostPort(host, port) == endpoint {
			return true
		}
	}
	return false
}

func macError(value *C.char) error {
	if value == nil {
		return errors.New("无法读取 macOS 系统代理")
	}
	defer C.free(unsafe.Pointer(value))
	switch message := C.GoString(value); message {
	case "network-service-missing":
		return errGone
	case "proxy-settings-changed":
		return errChanged
	default:
		return errors.New(message)
	}
}
