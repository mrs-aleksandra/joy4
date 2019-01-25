package main

import (
	"fmt"
	"io"
	"net/http"
	"sync"

	"github.com/nareix/joy4/av/avutil"
	"github.com/nareix/joy4/av/pubsub"
	"github.com/nareix/joy4/format"
	"github.com/nareix/joy4/format/flv"
	"github.com/nareix/joy4/format/rtmp"
)

func init() {
	format.RegisterAll()
}

type writeFlusher struct {
	httpflusher http.Flusher
	io.Writer
}

func (self writeFlusher) Flush() error {
	self.httpflusher.Flush()
	return nil
}

func main() {
	server := &rtmp.Server{}

	l := &sync.RWMutex{}
	type Channel struct {
		que *pubsub.Queue
	}
	channels := map[string]*Channel{}

	server.HandlePlay = func(conn *rtmp.Conn) {
		l.RLock()
		ch := channels[conn.URL.Path]
		l.RUnlock()

		fmt.Println("HandlePlay", conn.URL.Path, ch, channels)

		if ch != nil {
			cursor := ch.que.Latest()
			avutil.CopyFile(conn, cursor)
		}

		fmt.Println("HandlePlay finish", conn.URL.Path, ch, channels)
	}

	server.HandlePublish = func(conn *rtmp.Conn) {
		streams, _ := conn.Streams()

		l.Lock()
		ch := channels[conn.URL.Path]
		if ch == nil {
			ch = &Channel{}
			ch.que = pubsub.NewQueue()
			ch.que.WriteHeader(streams)
			channels[conn.URL.Path] = ch
		} else {
			ch = nil
		}
		l.Unlock()
		if ch == nil {
			return
		}

		fmt.Println("HandlePublish", conn.URL.Path, ch, channels)

		avutil.CopyPackets(ch.que, conn)

		fmt.Println("CopyPackets", conn.URL.Path, ch, channels)

		l.Lock()
		delete(channels, conn.URL.Path)
		l.Unlock()
		ch.que.Close()

		fmt.Println("Close", conn.URL.Path, ch, channels)
	}

	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		fmt.Println(r.URL.Path)
		l.RLock()
		ch := channels[r.URL.Path]
		l.RUnlock()

		fmt.Println(r.URL.Path, ch)

		if ch != nil {
			w.Header().Set("Content-Type", "video/x-flv")
			w.Header().Set("Transfer-Encoding", "chunked")
			w.Header().Set("Access-Control-Allow-Origin", "*")
			w.WriteHeader(200)
			flusher := w.(http.Flusher)
			flusher.Flush()

			muxer := flv.NewMuxerWriteFlusher(writeFlusher{httpflusher: flusher, Writer: w})
			cursor := ch.que.Latest()

			fmt.Println("HandleFunc", cursor)

			avutil.CopyFile(muxer, cursor)
		} else {
			http.NotFound(w, r)
		}
	})

	go http.ListenAndServe(":8080", nil)

	server.ListenAndServe()

	// ffmpeg -re -i manuel.mp4 -c copy -f flv rtmp://localhost/movie
	// ffmpeg -f avfoundation -i "0:0" .... -f flv rtmp://localhost/screen
	// ffmpeg -f avfoundation -i "0:0" -f flv rtmp://localhost/screen

	// default camera low quality
	// NOTE: "0:0" means default camera and audio, specify size, video and audio codec
	// ffmpeg -f avfoundation -i "0:0" -s 768x432 -vcodec h264 -acodec mp2 out.mp4
	// ffmpeg -f avfoundation -i "0:0" -s 768x432 -f flv rtmp://localhost/screen

	// ffplay http://localhost:8088/movie
	// ffplay http://localhost:8088/screen
}
