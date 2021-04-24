package biz

import (
	"encoding/json"
	"fmt"
	log "github.com/pion/ion-log"
	"github.com/pion/ion/pkg/grpc/ion"
	biz "github.com/pion/ion/pkg/node/biz/model"
	"net/http"
	"time"
)

type InfoServer struct {
	Biz *BIZ
}

type RoomInfo struct {
	ChatHistory *RoomChatHistory
	Host        *Peer
	Partners    []*Peer
}

func NewRoomInfo() *RoomInfo {
	return &RoomInfo{
		ChatHistory: NewRoomChatHistory(),
	}
}

type RoomChatHistory struct {
	Messages []*RoomMessage `json:"messages"`
}

const MAX_CHAT_HISTORY = 5

// addMessage call when new message send to room
func (history *RoomChatHistory) addMessage(message *RoomMessage) {
	if message.Peer == nil {
		return
	}
	history.Messages = append(history.Messages, message)
	if len(history.Messages) > MAX_CHAT_HISTORY {
		history.Messages = history.Messages[1:]
	}
	log.Debugf("RoomChatHistory len = %v", len(history.Messages))
}

func NewRoomChatHistory() *RoomChatHistory {
	return &RoomChatHistory{
		Messages: make([]*RoomMessage, 0),
	}
}

type RoomMessage struct {
	Message  *ion.Message `json:"-"`
	UnixNano int64        `json:"unixNano"`
	Data     string       `json:"data"`
	Peer     *Peer
}

type PeerRes struct {
}

func NewRoomMessage(msg *ion.Message, peer *Peer) *RoomMessage {
	return &RoomMessage{
		Peer:     peer,
		UnixNano: time.Now().UnixNano(),
		Message:  msg,
		Data:     string(msg.Data),
	}
}

type RoomInfoRes struct {
	Sid         string           `json:"sid"`
	Count       int              `json:"count"`
	ChatHistory *RoomChatHistory `json:"chats"`
	Host        *PeerInfo        `json:"host"`
	Partners    []*PeerInfo      `json:"partners"`
}

type PeerInfo struct {
	Uuid     string            `json:"uuid"`
	Birth    int               `json:"birth"`
	Nickname string            `json:"nickname"`
	Gender   int               `json:"gender"`
}

func NewPeerInfo() *PeerInfo{
	return &PeerInfo{
		Uuid: "1231",
		Birth: 1029,
		Nickname: "Dante",
		Gender: 1,
	}
}

func (p *Peer) toJson() *PeerInfo {
	if p == nil {
		return nil
	}
	return p.peerInfo
}

func (r *Room) toJson() RoomInfoRes {
	Partners := make([]*PeerInfo,0)
	for _, p := range r.roomInfo.Partners {
		if p != nil {
			Partners = append(Partners, p.peerInfo)
		}
	}
	res := RoomInfoRes{
		Sid:         r.sid,
		Count:       r.count(),
		ChatHistory: r.roomInfo.ChatHistory,
		Host:        r.roomInfo.Host.toJson(),
		Partners: Partners,
	}
	return res
}

func (info *InfoServer) StartServer() {
	infoServer := http.NewServeMux()
	infoServer.HandleFunc("/", func(resp http.ResponseWriter, req *http.Request) {
		log.Infof("Endpoint Hit: returnAllArticles")
		rooms := info.getListRoomInfo()
		err := json.NewEncoder(resp).Encode(rooms)
		if err != nil {
			log.Errorf(err.Error())
		}
	})
	infoServer.HandleFunc("/test", func(resp http.ResponseWriter, req *http.Request) {
		log.Infof("Endpoint Hit: test")
		db := biz.GetDb()
		if db != nil {
			biz.InitDbSchema(db)
		}
		_, _ = fmt.Fprintf(resp, "Hello, %v", 123123)
	})
	go func() {
		addr := "0.0.0.0:3095"
		err := http.ListenAndServe(addr, infoServer)
		if err != nil {
			log.Errorf("Failed to start Info Server on : %v err: %v", addr, err)
		}
	}()
}

func (info *InfoServer) getListRoomInfo() []RoomInfoRes {
	var biz *BizServer = info.Biz.s
	var rooms = make([]RoomInfoRes, 0)
	biz.roomLock.RLock()
	for _, room := range biz.rooms {
		room.toJson()
		rooms = append(rooms, room.toJson())
	}
	biz.roomLock.RUnlock()
	return rooms
}
