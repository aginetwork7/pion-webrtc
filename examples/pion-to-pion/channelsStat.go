package monitor

import (
	"fmt"
	"sync"
	"time"
)

type MonitorDirection int

const (
	Send MonitorDirection = 1 << iota
	Recv
	Both = Send | Recv
)

type DataChannelMonitor struct {
	interval  time.Duration
	window    int
	direction MonitorDirection

	mu       sync.Mutex
	sendSum  uint64
	recvSum  uint64
	sendRate []float64
	recvRate []float64
	stopCh   chan struct{}
}

func NewMonitor(interval time.Duration, window int, dir MonitorDirection) *DataChannelMonitor {
	return &DataChannelMonitor{
		interval:  interval,
		window:    window,
		direction: dir,
		sendRate:  make([]float64, 0, window),
		recvRate:  make([]float64, 0, window),
		stopCh:    make(chan struct{}),
	}
}

func (m *DataChannelMonitor) RecordSendBytes(n int) {
	if m.direction&Send == 0 {
		return
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.sendSum += uint64(n)
}

func (m *DataChannelMonitor) RecordRecvBytes(n int) {
	if m.direction&Recv == 0 {
		return
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.recvSum += uint64(n)
}

func (m *DataChannelMonitor) Start() {
	ticker := time.NewTicker(m.interval)
	go func() {
		for {
			select {
			case <-ticker.C:
				m.mu.Lock()
				sendBytes := m.sendSum
				recvBytes := m.recvSum
				m.sendSum = 0
				m.recvSum = 0
				m.mu.Unlock()

				sendRate := float64(sendBytes*8) / m.interval.Seconds() / 1e6 // Mbps
				recvRate := float64(recvBytes*8) / m.interval.Seconds() / 1e6

				if m.direction&Send != 0 {
					m.sendRate = append(m.sendRate, sendRate)
					if len(m.sendRate) > m.window {
						m.sendRate = m.sendRate[1:]
					}
					fmt.Printf("[DataChannel] SEND: %.2f Mbps\n", sendRate)
				}
				if m.direction&Recv != 0 {
					m.recvRate = append(m.recvRate, recvRate)
					if len(m.recvRate) > m.window {
						m.recvRate = m.recvRate[1:]
					}
					fmt.Printf("[DataChannel] RECV: %.2f Mbps\n", recvRate)
				}
			case <-m.stopCh:
				ticker.Stop()
				return
			}
		}
	}()
}

func (m *DataChannelMonitor) Stop() {
	close(m.stopCh)
}
