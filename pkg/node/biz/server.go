package biz

import (
	"context"
	"fmt"
	"io"
	"sync"
	"time"

	nrpc "github.com/cloudwebrtc/nats-grpc/pkg/rpc"
	"github.com/nats-io/nats.go"
	log "github.com/pion/ion-log"
	biz "github.com/pion/ion/pkg/grpc/biz"
	islb "github.com/pion/ion/pkg/grpc/islb"
	"github.com/pion/ion/pkg/proto"
	"github.com/pion/ion/pkg/util"
)

// BizServer represents an BizServer instance
type BizServer struct {
	biz.UnimplementedBizServer
	nc       *nats.Conn
	elements []string
	roomLock sync.RWMutex
	rooms    map[string]*Room
	closed   chan bool
	islbcli  islb.ISLBClient
	bn       *BIZ
	stream   islb.ISLB_WatchISLBEventClient
}

// newBizServer creates a new avp server instance
func newBizServer(bn *BIZ, c string, nid string, elements []string, nc *nats.Conn) *BizServer {
	return &BizServer{
		bn:       bn,
		nc:       nc,
		elements: elements,
		rooms:    make(map[string]*Room),
		closed:   make(chan bool),
		stream:   nil,
	}
}

func (s *BizServer) close() {
	close(s.closed)
}

func (s *BizServer) createRoom(sid string, sfuNID string) *Room {
	s.roomLock.RLock()
	defer s.roomLock.RUnlock()
	r := newRoom(sid, sfuNID)
	s.rooms[sid] = r
	return r
}

func (s *BizServer) getRoom(id string) *Room {
	s.roomLock.Lock()
	defer s.roomLock.Unlock()
	r := s.rooms[id]
	return r
}

func (s *BizServer) delRoom(id string) {
	s.roomLock.Lock()
	defer s.roomLock.Unlock()
	delete(s.rooms, id)
}

func (s *BizServer) watchISLBEvent(nid string, sid string) error {

	if s.stream == nil {
		stream, err := s.islbcli.WatchISLBEvent(context.Background())
		if err != nil {
			return err
		}
		err = stream.Send(&islb.WatchRequest{
			Nid: nid,
			Sid: sid,
		})
		if err != nil {
			return err
		}

		go func() {
			for {
				req, err := stream.Recv()
				if err != nil {
					log.Errorf("BizServer.Singal server stream.Recv() err: %v", err)
					return
				}
				log.Infof("watchISLBEvent req => %v", req)
				switch payload := req.Payload.(type) {
				case *islb.ISLBEvent_Stream:
					r := s.getRoom(payload.Stream.Sid)
					if r != nil {
						r.sendStreamEvent(payload.Stream)
						p := r.getPeer(payload.Stream.Uid)
						// save last stream info.
						if p != nil {
							p.lastStreamEvent = payload.Stream
						}
					}
				}
			}
		}()
	}
	return nil
}

//Signal process biz request.
func (s *BizServer) Signal(stream biz.Biz_SignalServer) error {
	var r *Room = nil
	var peer *Peer = nil
	errCh := make(chan error)
	repCh := make(chan *biz.SignalReply)
	reqCh := make(chan *biz.SignalRequest)

	defer func() {
		if peer != nil && r != nil {
			peer.Close()
			r.delPeer(peer.UID())
		}

		if r != nil && r.count() == 0 {
			s.delRoom(r.SID())
			r = nil
		}

		log.Infof("BizServer.Signal loop done")
	}()

	go func() {
		for {
			// get the signal request from client
			req, err := stream.Recv()
			if err != nil {
				log.Errorf("BizServer.Singal server stream.Recv() err: %v", err)
				errCh <- err
				return
			}
			reqCh <- req
		}
	}()

	for {
		select {
		case err := <-errCh:
			// Error occur -> close loop
			return err
		case reply, ok := <-repCh:
			if !ok {
				return io.EOF
			}
			err := stream.Send(reply)
			if err != nil {
				return err
			}
		case req, ok := <-reqCh:
			if !ok {
				return io.EOF
			}
			log.Infof("Biz request => %v", req.String())

			switch payload := req.Payload.(type) {
			case *biz.SignalRequest_Join:
				sid := payload.Join.Peer.Sid
				uid := payload.Join.Peer.Uid

				success := false
				reason := "unkown error."

				if s.islbcli == nil {
					nodes := s.bn.GetNeighborNodes()
					for _, node := range nodes {
						if node.Service == proto.ServiceISLB {
							ncli := nrpc.NewClient(s.nc, node.NID)
							s.islbcli = islb.NewISLBClient(ncli)
							break
						}
					}
				}

				if s.islbcli != nil {
					r = s.getRoom(sid)
					if r == nil {
						reason = fmt.Sprintf("room sid = %v not found", sid)
						// Ask ISLB process to get SFU node
						resp, err := s.islbcli.FindNode(context.TODO(), &islb.FindNodeRequest{
							Service: proto.ServiceSFU,
							Sid:     sid,
						})
						nid := ""
						if err == nil && len(resp.Nodes) > 0 {
							// Found SFU node -> create room
							nid = resp.GetNodes()[0].Nid
							r = s.createRoom(sid, nid)
						} else {
							// Not found SFU node -> SFU server is down, err maybe EOF
							reason = fmt.Sprintf("islbcli.FindNode(serivce = sfu, sid = %v) err %v", sid, err)
						}

						err = s.watchISLBEvent(nid, sid)
						if err != nil {
							log.Errorf("s.watchISLBEvent(req) failed %v", err)
						}
					}
					if r != nil {
						// get or created room -> create Peer
						peer = NewPeer(sid, uid, payload.Join.Peer.Info, repCh)
						r.addPeer(peer)
						success = true
						reason = "join success."
						// TODO: Test
						r.roomInfo.Host = peer
						r.roomInfo.Partners = append(r.roomInfo.Partners, peer)
					} else {
						log.Errorf("Room not found or failed to create sid = %v", sid)
					}
				} else {
					reason = fmt.Sprintf("join [sid=%v] islb node not found", sid)
				}

				err := stream.Send(&biz.SignalReply{
					Payload: &biz.SignalReply_JoinReply{
						JoinReply: &biz.JoinReply{
							Success: success,
							Reason:  reason,
						},
					},
				})

				if err != nil {
					log.Errorf("stream.Send(&biz.SignalReply) failed %v", err)
				}
			case *biz.SignalRequest_Leave:
				uid := payload.Leave.Uid
				if peer != nil && peer.uid == uid {
					r.delPeer(uid)
					peer.Close()
					peer = nil

					if r.count() == 0 {
						s.delRoom(r.SID())
						r = nil
					}

					err := stream.Send(&biz.SignalReply{
						Payload: &biz.SignalReply_LeaveReply{
							LeaveReply: &biz.LeaveReply{
								Reason: "closed",
							},
						},
					})
					if err != nil {
						log.Errorf("stream.Send(&biz.SignalReply) failed %v", err)
					}
				}
			case *biz.SignalRequest_Msg:
				log.Debugf("Message: from: %v => to: %v, data: %v", payload.Msg.From, payload.Msg.To, payload.Msg.Data)
				// message broadcast
				if r != nil {
					r.roomInfo.ChatHistory.addMessage(NewRoomMessage(payload.Msg, peer))
					r.sendMessage(payload.Msg)
				} else {
					log.Warnf("room not found, maybe the peer did not join")
				}
			default:
				break
			}

		}
	}
}

// stat peers
func (s *BizServer) stat() {
	t := time.NewTicker(util.DefaultStatCycle)
	defer t.Stop()
	for {
		select {
		case <-t.C:
		case <-s.closed:
			log.Infof("stop stat")
			return
		}

		var info string
		s.roomLock.RLock()
		for sid, room := range s.rooms {
			info += fmt.Sprintf("room: %s\npeers: %d\n", sid, room.count())
		}
		s.roomLock.RUnlock()
		if len(info) > 0 {
			log.Infof("\n----------------signal-----------------\n" + info)
		}
	}
}
