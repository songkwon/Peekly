package systemproxy

import (
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"sync"
)

var (
	errChanged = errors.New("系统代理已由其他应用更改")
	errGone    = errors.New("原网络服务已移除")
)

type fields map[string]any

type record struct {
	Service  string `json:"service"`
	Endpoint string `json:"endpoint"`
	Before   fields `json:"before"`
	After    fields `json:"after"`
}

type driver interface {
	prepare(address string, port int) (record, error)
	current(service string) (fields, error)
	replace(service string, from, to fields, allowPrompt bool) error
	references(current fields, endpoint string) bool
}

type Status struct {
	Active  bool `json:"active"`
	Pending bool `json:"pending"`
}

type Manager struct {
	mu     sync.Mutex
	path   string
	driver driver
	active bool
}

func New(dir string) *Manager {
	return &Manager{path: filepath.Join(dir, "system-proxy-recovery.json"), driver: newDriver()}
}

func (m *Manager) Status() Status {
	m.mu.Lock()
	defer m.mu.Unlock()
	_, err := os.Stat(m.path)
	return Status{Active: m.active, Pending: !m.active && !errors.Is(err, os.ErrNotExist)}
}

func (m *Manager) Enable(endpoint string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.active {
		return nil
	}
	if err := m.restore(false); err != nil {
		return fmt.Errorf("请先恢复上次的系统代理: %w", err)
	}
	host, value, err := net.SplitHostPort(endpoint)
	port, parseErr := strconv.Atoi(value)
	if err != nil || parseErr != nil || host != "127.0.0.1" || port < 1024 || port > 65535 {
		return errors.New("自动配置需要先在本机回环地址启动 Peekly")
	}
	plan, err := m.driver.prepare(host, port)
	if err != nil {
		return err
	}
	plan.Endpoint = endpoint
	if m.driver.references(plan.Before, endpoint) {
		return errors.New("系统代理已手动指向此端口。请关闭这条手动配置后再选择自动配置")
	}
	if err = m.save(plan); err != nil {
		return fmt.Errorf("保存原代理设置失败: %w", err)
	}
	if err = m.driver.replace(plan.Service, plan.Before, plan.After, true); err == nil {
		m.active = true
		return nil
	}
	rollback := m.restore(false)
	return errors.Join(err, rollback)
}

func (m *Manager) Restore(allowPrompt bool) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.restore(allowPrompt)
}

func (m *Manager) restore(allowPrompt bool) error {
	data, err := os.ReadFile(m.path)
	if errors.Is(err, os.ErrNotExist) {
		m.active = false
		return nil
	}
	if err != nil {
		return err
	}
	var plan record
	if err = json.Unmarshal(data, &plan); err != nil {
		return fmt.Errorf("读取代理恢复记录失败: %w", err)
	}
	if plan.Service == "" || plan.Endpoint == "" || len(plan.Before) == 0 || len(plan.After) == 0 {
		return errors.New("代理恢复记录不完整")
	}
	current, err := m.driver.current(plan.Service)
	if errors.Is(err, errGone) {
		return m.clear()
	}
	if err != nil {
		return err
	}
	if equal(current, plan.Before) || !m.driver.references(current, plan.Endpoint) {
		return m.clear()
	}
	if !equal(current, plan.After) {
		return errors.New("系统代理仍指向 Peekly，但配置已在外部更改。请检查系统代理设置后重试")
	}
	if err := m.driver.replace(plan.Service, current, plan.Before, allowPrompt); err != nil {
		return fmt.Errorf("恢复系统代理失败: %w", err)
	}
	return m.clear()
}

func (m *Manager) clear() error {
	if err := os.Remove(m.path); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	m.active = false
	return nil
}

func (m *Manager) save(plan record) error {
	parent := filepath.Dir(m.path)
	if err := os.MkdirAll(parent, 0700); err != nil {
		return err
	}
	file, err := os.CreateTemp(parent, ".system-proxy-*")
	if err != nil {
		return err
	}
	defer os.Remove(file.Name())
	err = json.NewEncoder(file).Encode(plan)
	if err == nil {
		err = file.Sync()
	}
	if err = errors.Join(err, file.Close()); err != nil {
		return err
	}
	return os.Link(file.Name(), m.path)
}

func equal(a, b fields) bool {
	left, _ := json.Marshal(a)
	right, _ := json.Marshal(b)
	return string(left) == string(right)
}
