package systemproxy

import (
	"errors"
	"fmt"
	"net"
	"runtime"
	"strconv"
	"strings"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"
)

const (
	optionPerConnection        = 75
	optionRefresh              = 37
	optionProxySettingsChanged = 95
	connFlags                  = 1
	connServer                 = 2
	connBypass                 = 3
	connFlagsUI                = 10
	proxyDirect                = 1
	proxyEnabled               = 2
)

var (
	wininet     = windows.NewLazySystemDLL("wininet.dll")
	queryOption = wininet.NewProc("InternetQueryOptionW")
	setOption   = wininet.NewProc("InternetSetOptionW")
	globalFree  = windows.NewLazySystemDLL("kernel32.dll").NewProc("GlobalFree")
)

type option struct {
	id    uint32
	value uintptr
}

type optionList struct {
	size       uint32
	connection *uint16
	count      uint32
	errorIndex uint32
	options    *option
}

type windowsDriver struct{}

func newDriver() driver { return windowsDriver{} }

func (d windowsDriver) prepare(host string, port int) (record, error) {
	before, err := d.current("LAN")
	if err != nil {
		return record{}, err
	}
	bypass, _ := before["bypass"].(string)
	if !strings.Contains(strings.ToLower(bypass), "wails.localhost") {
		bypass = strings.Trim(strings.TrimSpace(bypass), ";")
		if bypass != "" {
			bypass += ";"
		}
		bypass += "wails.localhost"
	}
	endpoint := net.JoinHostPort(host, strconv.Itoa(port))
	return record{Service: "LAN", Before: before, After: fields{"flags": proxyDirect | proxyEnabled, "server": endpoint, "bypass": bypass}}, nil
}

func (windowsDriver) current(service string) (fields, error) {
	if service != "LAN" {
		return nil, errGone
	}
	settings, err := queryWindows(connFlagsUI)
	if err != nil {
		return queryWindows(connFlags)
	}
	return settings, nil
}

func queryWindows(flags uint32) (fields, error) {
	options := []option{{id: flags}, {id: connServer}, {id: connBypass}}
	list := optionList{size: uint32(unsafe.Sizeof(optionList{})), count: uint32(len(options)), options: &options[0]}
	size := list.size
	ok, _, err := queryOption.Call(0, optionPerConnection, uintptr(unsafe.Pointer(&list)), uintptr(unsafe.Pointer(&size)))
	for _, item := range options[1:] {
		if item.value != 0 {
			defer globalFree.Call(item.value)
		}
	}
	if ok == 0 {
		return nil, winError("读取 Windows 系统代理", err)
	}
	asString := func(pointer uintptr) string {
		if pointer == 0 {
			return ""
		}
		return windows.UTF16PtrToString((*uint16)(unsafe.Pointer(pointer)))
	}
	return fields{"flags": uint32(options[0].value), "server": asString(options[1].value), "bypass": asString(options[2].value)}, nil
}

func (d windowsDriver) replace(service string, expected, next fields, _ bool) error {
	current, err := d.current(service)
	if err != nil {
		return err
	}
	if !equal(expected, current) {
		return errChanged
	}
	server, _ := next["server"].(string)
	bypass, _ := next["bypass"].(string)
	flags := flagsOf(next)
	serverText, err := windows.UTF16PtrFromString(server)
	if err != nil {
		return err
	}
	bypassText, err := windows.UTF16PtrFromString(bypass)
	if err != nil {
		return err
	}
	options := []option{
		{id: connFlags, value: uintptr(flags)},
		{id: connServer, value: uintptr(unsafe.Pointer(serverText))},
		{id: connBypass, value: uintptr(unsafe.Pointer(bypassText))},
	}
	list := optionList{size: uint32(unsafe.Sizeof(optionList{})), count: uint32(len(options)), options: &options[0]}
	ok, _, err := setOption.Call(0, optionPerConnection, uintptr(unsafe.Pointer(&list)), uintptr(list.size))
	runtime.KeepAlive(serverText)
	runtime.KeepAlive(bypassText)
	if ok == 0 {
		return winError("更新 Windows 系统代理", err)
	}
	setOption.Call(0, optionProxySettingsChanged, 0, 0)
	setOption.Call(0, optionRefresh, 0, 0)
	return nil
}

func flagsOf(state fields) uint32 {
	number, _ := strconv.ParseUint(fmt.Sprint(state["flags"]), 10, 32)
	return uint32(number)
}

func (windowsDriver) references(state fields, endpoint string) bool {
	server, _ := state["server"].(string)
	if flagsOf(state)&proxyEnabled == 0 {
		return false
	}
	for _, entry := range strings.Split(server, ";") {
		_, address, ok := strings.Cut(entry, "=")
		if !ok {
			address = entry
		}
		address = strings.TrimPrefix(strings.TrimPrefix(strings.TrimSpace(address), "http://"), "https://")
		if strings.EqualFold(address, endpoint) {
			return true
		}
	}
	return false
}

func winError(activity string, err error) error {
	if err == nil || errors.Is(err, syscall.Errno(0)) {
		return errors.New(activity + "失败")
	}
	return fmt.Errorf("%s: %w", activity, err)
}
