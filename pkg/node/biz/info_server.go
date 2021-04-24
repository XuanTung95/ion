package biz

import (
	"encoding/json"
	log "github.com/pion/ion-log"
	"net/http"
)

// TestData - Our struct for all articles
type TestData struct {
	Id      string    `json:"Id"`
	Title   string `json:"Title"`
	Desc    string `json:"desc"`
	Content string `json:"content"`
	Count   int    `json:"count"`
}

func (r *Room) toJson() TestData {
	res := TestData{}
	res.Id = r.sid
	res.Title = r.sfunid
	res.Count = r.count()
	return res
}

func (s *BizServer) StartInfoServer() {
	infoServer := http.NewServeMux()
	infoServer.HandleFunc("/", func(resp http.ResponseWriter, req *http.Request) {
		log.Infof("Endpoint Hit: test API")
		var TestRes []TestData
		TestRes = append(TestRes, TestData{
			Title: "Test1",
			Desc: "Test2",
		})
		s.roomLock.RLock()
		for _, room := range s.rooms {
			room.toJson()
			TestRes = append(TestRes, room.toJson())
		}
		s.roomLock.RUnlock()
		err := json.NewEncoder(resp).Encode(TestRes)
		if err != nil {
			log.Errorf(err.Error())
		}
	})
	go func() {
		addr := "0.0.0.0:3095"
		err := http.ListenAndServe(addr, infoServer)
		if err != nil {
			log.Errorf("Failed to start Info Server on : %v err: %v", addr, err)
		}
	}()
}

