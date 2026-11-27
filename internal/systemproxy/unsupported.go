//go:build !windows && (!darwin || !cgo)

package systemproxy

import "errors"

type unsupported struct{}

func newDriver() driver { return unsupported{} }
func (unsupported) prepare(string, int) (record, error) {
	return record{}, errors.New("此平台不支持自动配置系统代理")
}
func (unsupported) current(string) (fields, error)             { return nil, errGone }
func (unsupported) replace(string, fields, fields, bool) error { return errGone }
func (unsupported) references(fields, string) bool             { return false }
