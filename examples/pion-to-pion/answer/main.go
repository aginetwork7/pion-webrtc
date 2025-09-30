// SPDX-FileCopyrightText: 2023 The Pion community <https://pion.ly>
// SPDX-License-Identifier: MIT

// pion-to-pion is an example of two pion instances communicating directly!
package main

import (
	"bytes"
	"crypto/rand"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"sync"
	"time"

	"github.com/pion/datachannel"
	"github.com/pion/dtls/v3"
	"github.com/pion/dtls/v3/pkg/crypto/customercryptociphersuite"
	"github.com/pion/webrtc/v4"
	monitor "github.com/pion/webrtc/v4/examples/pion-to-pion"
)

func signalCandidate(addr string, c *webrtc.ICECandidate) error {
	payload := []byte(c.ToJSON().Candidate)
	resp, err := http.Post(fmt.Sprintf("http://%s/candidate", addr), // nolint:noctx
		"application/json; charset=utf-8", bytes.NewReader(payload))
	if err != nil {
		return err
	}

	return resp.Body.Close()
}

var monitorwin = monitor.NewMonitor(time.Second, 30, monitor.Send)
var writable = make(chan struct{}, 1)

func main() { // nolint:gocognit
	offerAddr := flag.String("offer-address", "127.0.0.1:50000", "Address that the Offer HTTP server is hosted on.")
	answerAddr := flag.String("answer-address", ":60000", "Address that the Answer HTTP server is hosted on.")
	flag.Parse()

	var candidatesMux sync.Mutex
	pendingCandidates := make([]*webrtc.ICECandidate, 0)
	// Everything below is the Pion WebRTC API! Thanks for using it ❤️.

	s := webrtc.SettingEngine{}

	config := webrtc.Configuration{
		ICEServers: []webrtc.ICEServer{
			{
				URLs: []string{"stun:stun.l.google.com:19302"},
			},
		},
	}

	callback := func() []dtls.CipherSuite {
		cs := &customercryptociphersuite.TLSEcdheRsaWithChaCha20Poly1305Sha256{}
		masterSecret := make([]byte, 48)
		clientRandom := make([]byte, 32)
		serverRandom := make([]byte, 32)

		cs.Init(masterSecret, clientRandom, serverRandom, true)

		return []dtls.CipherSuite{cs}
	}
	s.SetDTLSCustomerCipherSuites(callback)

	//s.DetachDataChannels()
	api := webrtc.NewAPI(webrtc.WithSettingEngine(s))
	peerConnection, err := api.NewPeerConnection(config)
	if err != nil {
		panic(err)
	}

	defer func() {
		if err := peerConnection.Close(); err != nil {
			fmt.Printf("cannot close peerConnection: %v\n", err)
		}
	}()

	// When an ICE candidate is available send to the other Pion instance
	// the other Pion instance will add this candidate by calling AddICECandidate
	peerConnection.OnICECandidate(func(c *webrtc.ICECandidate) {
		if c == nil {
			return
		}

		candidatesMux.Lock()
		defer candidatesMux.Unlock()

		desc := peerConnection.RemoteDescription()
		if desc == nil {
			pendingCandidates = append(pendingCandidates, c)
		} else if onICECandidateErr := signalCandidate(*offerAddr, c); onICECandidateErr != nil {
			panic(onICECandidateErr)
		}
	})

	// A HTTP handler that allows the other Pion instance to send us ICE candidates
	// This allows us to add ICE candidates faster, we don't have to wait for STUN or TURN
	// candidates which may be slower
	http.HandleFunc("/candidate", func(w http.ResponseWriter, r *http.Request) {
		candidate, candidateErr := io.ReadAll(r.Body)
		if candidateErr != nil {
			panic(candidateErr)
		}
		if candidateErr := peerConnection.AddICECandidate(webrtc.ICECandidateInit{Candidate: string(candidate)}); candidateErr != nil {
			panic(candidateErr)
		}
	})

	// A HTTP handler that processes a SessionDescription given to us from the other Pion process
	http.HandleFunc("/sdp", func(w http.ResponseWriter, r *http.Request) {
		sdp := webrtc.SessionDescription{}
		if err := json.NewDecoder(r.Body).Decode(&sdp); err != nil {
			panic(err)
		}

		if err := peerConnection.SetRemoteDescription(sdp); err != nil {
			panic(err)
		}
		peerConnection.CreateDataChannel("data", nil)
		// Create an answer to send to the other process
		answer, err := peerConnection.CreateAnswer(nil)
		if err != nil {
			panic(err)
		}

		var tmpAnwser = answer
		if tmpAnwser.SDP != "" {
			tmpAnwser.SDP += "a=max-message-size:262144\r\n"
		}
		// Send our answer to the HTTP server listening in the other process
		payload, err := json.Marshal(tmpAnwser)
		if err != nil {
			panic(err)
		}
		resp, err := http.Post(fmt.Sprintf("http://%s/sdp", *offerAddr), "application/json; charset=utf-8", bytes.NewReader(payload)) // nolint:noctx
		if err != nil {
			panic(err)
		} else if closeErr := resp.Body.Close(); closeErr != nil {
			panic(closeErr)
		}
		// Sets the LocalDescription, and starts our UDP listeners
		err = peerConnection.SetLocalDescription(answer)
		if err != nil {
			panic(err)
		}

		candidatesMux.Lock()
		for _, c := range pendingCandidates {
			onICECandidateErr := signalCandidate(*offerAddr, c)
			if onICECandidateErr != nil {
				panic(onICECandidateErr)
			}
		}
		candidatesMux.Unlock()
	})

	// Set the handler for Peer connection state
	// This will notify you when the peer has connected/disconnected
	peerConnection.OnConnectionStateChange(func(s webrtc.PeerConnectionState) {
		fmt.Printf("Peer Connection State has changed: %s\n", s.String())

		if s == webrtc.PeerConnectionStateFailed {
			// Wait until PeerConnection has had no network activity for 30 seconds or another failure. It may be reconnected using an ICE Restart.
			// Use webrtc.PeerConnectionStateDisconnected if you are interested in detecting faster timeout.
			// Note that the PeerConnection may come back from PeerConnectionStateDisconnected.
			fmt.Println("Peer Connection has gone to failed exiting")
			os.Exit(0)
		}

		if s == webrtc.PeerConnectionStateClosed {
			// PeerConnection was explicitly closed. This usually happens from a DTLS CloseNotify
			fmt.Println("Peer Connection has gone to closed exiting")
			os.Exit(0)
		}
	})

	// Register data channe	l creation handling
	peerConnection.OnDataChannel(func(d *webrtc.DataChannel) {
		fmt.Printf("New DataChannel %s %d\n", d.Label(), d.ID())

		d.SetBufferedAmountLowThreshold(1 * 1024 * 1024)
		d.OnBufferedAmountLow(func() {
			select {
			case writable <- struct{}{}:
			default:
			}
		})

		d.OnOpen(func() {
			monitorwin.Start()
			go WriteLoop(d)

		})

		// Register text message handling
		d.OnMessage(func(msg webrtc.DataChannelMessage) {
			fmt.Printf("Message from DataChannel '%s': '%s'\n", d.Label(), string(msg.Data))
		})
	})

	// Start HTTP server that accepts requests from the offer process to exchange SDP and Candidates
	// nolint: gosec
	panic(http.ListenAndServe(*answerAddr, nil))
}

func WriteLoop(d *webrtc.DataChannel) {
	raw, _ := d.Detach()
	if raw != nil {
		v, ok := raw.(*datachannel.DataChannel)
		if !ok {
			panic("类型转换失败")
		}
		fmt.Println("datachannel detached")
		WriteLoopV2(v)
	} else {
		buf := make([]byte, 32*1024)
		for {
			maxBufferAmount := uint64(2 * 1024 * 1024)
			if d.BufferedAmount() >= maxBufferAmount {
				<-writable
			}
			rand.Read(buf)
			if err := d.Send(buf); err != nil {
				panic(err)
			}
			monitorwin.RecordSendBytes(len(buf))
		}
	}
}

func WriteLoopV2(v *datachannel.DataChannel) {
	buf := make([]byte, 32*1024)
	for {
		maxBufferAmount := uint64(2 * 1024 * 1024)
		if v.BufferedAmount() >= maxBufferAmount {
			<-writable
		}
		rand.Read(buf)
		_, err2 := v.Write(buf)
		if err2 != nil {
			fmt.Println("发送失败: %v", err2)
			break
		}
		monitorwin.RecordSendBytes(len(buf))
	}
}
