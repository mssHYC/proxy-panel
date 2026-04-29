package kernel

import (
	"fmt"
	"log"
	"sync"
)

// Manager 管理多个内核引擎
type Manager struct {
	mu      sync.RWMutex
	engines map[string]Engine
}

// NewManager 创建引擎管理器
func NewManager() *Manager {
	return &Manager{
		engines: make(map[string]Engine),
	}
}

// Register 注册一个引擎
func (m *Manager) Register(engine Engine) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.engines[engine.Name()] = engine
}

// Get 根据名称获取引擎
func (m *Manager) Get(name string) (Engine, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	eng, ok := m.engines[name]
	if !ok {
		return nil, fmt.Errorf("引擎 %s 未注册", name)
	}
	return eng, nil
}

// Status 返回所有引擎的运行状态
func (m *Manager) Status() map[string]bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	result := make(map[string]bool, len(m.engines))
	for name, eng := range m.engines {
		result[name] = eng.IsRunning()
	}
	return result
}

// GetTrafficStats 合并所有运行中引擎的流量统计。单个引擎失败不阻断其他引擎。
//
// 同一 (用户, 节点) 在不同引擎都被采到时累加——理论上不会发生（同一节点只跑
// 一种内核），保留聚合是为了对极端配置（同 tag 复用）兜底，避免漏计。
func (m *Manager) GetTrafficStats() ([]TrafficStat, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	type key struct{ user, tag string }
	merged := make(map[key]*TrafficStat)
	for _, eng := range m.engines {
		if !eng.IsRunning() {
			continue
		}
		stats, err := eng.GetTrafficStats()
		if err != nil {
			log.Printf("[内核管理器] %s 流量采集失败: %v", eng.Name(), err)
			continue
		}
		for _, s := range stats {
			k := key{s.Username, s.NodeTag}
			if existing, ok := merged[k]; ok {
				existing.Upload += s.Upload
				existing.Download += s.Download
			} else {
				cp := s
				merged[k] = &cp
			}
		}
	}
	result := make([]TrafficStat, 0, len(merged))
	for _, st := range merged {
		result = append(result, *st)
	}
	return result, nil
}
