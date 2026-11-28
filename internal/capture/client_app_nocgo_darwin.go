//go:build darwin && !cgo

package capture

func processBundle(int) string                   { return "" }
func applicationDetails(string) (string, string) { return "", "" }
