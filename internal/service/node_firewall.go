package service

import (
	"context"
	"log"
)

type portAction uint8

const (
	allowPort portAction = iota
	revokePort
	ensurePort
)

// EnsureFirewallPorts rechecks a startup or settings snapshot before opening each port.
func (s *NodeService) EnsureFirewallPorts(ports []int) {
	for _, port := range ports {
		s.applyFirewallPort(port, ensurePort)
	}
}

func (s *NodeService) applyFirewallPort(port int, action portAction) {
	if !s.FirewallEnabled() {
		return
	}
	// Older async jobs may run after a newer save. Keep the ownership check
	// and the firewall command together so a new owner cannot appear between them.
	s.mutationMu.Lock()
	defer s.mutationMu.Unlock()
	var inUse bool
	if err := s.db.QueryRow("SELECT EXISTS(SELECT 1 FROM nodes WHERE enable = 1 AND port = ?)", port).Scan(&inUse); err != nil {
		log.Printf("[防火墙] 无法核对节点端口 %d，保留现有规则: %v", port, err)
		return
	}
	if action == revokePort {
		if inUse {
			return
		}
		if s.website != nil {
			websiteUsesPort, err := s.website.UsesPublicPort(port)
			if err != nil {
				log.Printf("[防火墙] 无法核对托管网站端口 %d，保留现有规则: %v", port, err)
				return
			}
			if websiteUsesPort {
				return
			}
		}
		s.fw.Revoke(port)
	} else if inUse {
		if action == ensurePort {
			s.fw.EnsureAll(context.Background(), []int{port})
		} else {
			s.fw.Allow(port)
		}
	}
}
