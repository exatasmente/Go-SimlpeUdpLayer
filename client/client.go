package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"net"
	"os"
	"strings"
)
type Message struct {
	Addr   string `json:"addr"`
	Action string `json:"action"`
	Status int32  `json:"status"`
	Data   []byte `json:"data"`
}

type action func (sockChan chan Message)

func main() {
	arguments := os.Args


	if len(arguments) == 1 {
		fmt.Println("Please provide host:port.")
		return
	}

	c, err := net.Dial("udp",arguments[1])
	if err != nil {
		fmt.Println(err)
		return
	}
	var done = make(chan bool)
	var writeChan = make(chan Message)
	defer close(done)
	defer  close(writeChan)
	defer c.Close()

	go func() {

		for {
			buf := make([]byte, 1024)
			n, err := c.Read(buf)

			if err != nil {
				continue
			}
			message := string(buf[:n])

			if err != nil {
				fmt.Errorf(err.Error())
				return
			}

			if len(message) > 0 {
				fmt.Println("\n->: " + message)
			}

		}
	}()

	go func() {
		reader := bufio.NewReader(os.Stdin)
		for {
			fmt.Print(">> ")
			text, _ := reader.ReadBytes('\n')

			if strings.TrimSpace(string(text)) == "STOP" {
				fmt.Println("TCP client exiting...")
				done <- true
				return
			}

			m := Message{
				Addr:   c.LocalAddr().String(),
				Status: 0,
				Data:   text[0 :len(text)-1],
			}

			writeChan <- m

		}


	}()

	var stop bool = false
	for {

		if stop {
			break
		}
		select {
			case <-done:
				stop = true
				fmt.Println("PAROu")
				break
			case data := <- writeChan:
				bytesData, _ := json.Marshal(data)
				c.Write(bytesData)
				break
		}
	}
	done <- true
}
