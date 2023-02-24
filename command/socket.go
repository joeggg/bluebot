package command

import (
	"encoding/json"
	"fmt"
	"log"

	zmq "github.com/pebbe/zmq4"
)

type WorkerResponse struct {
	Code   int               `json:"code"`
	Result interface{}       `json:"result"`
	Error  map[string]string `json:"error,omitempty"`
}

type WorkerSocket struct {
	sck *zmq.Socket
}

func NewWorkerSocket() (*WorkerSocket, error) {
	//  Prepare our context and sockets
	sck, err := zmq.NewSocket(zmq.REQ)
	if err != nil {
		log.Printf("Failed to create socket: %s", err)
		return nil, err
	}

	err = sck.Connect("tcp://localhost:5678")
	if err != nil {
		log.Printf("Failed to connect: %s", err)
		return nil, err
	}
	return &WorkerSocket{sck}, err
}

func (w *WorkerSocket) Send(command string, params *map[string]interface{}) error {
	log.Printf("Sending command %s to worker", command)
	data, err := json.Marshal(params)
	if err != nil {
		return err
	}
	_, err = w.sck.SendMessage(fmt.Sprintf(`{"command": "%s", "params": %s}`, command, data))
	return err
}

func (w *WorkerSocket) Receive() (interface{}, error) {
	log.Println("Waiting for a response from worker")
	data, err := w.sck.RecvMessage(zmq.SNDMORE)
	if err != nil {
		return nil, err
	}
	var response WorkerResponse
	err = json.Unmarshal([]byte(data[0]), &response)
	if err != nil {
		return nil, err
	}
	if response.Code != 0 {
		log.Printf("Worker errored: %s", response.Error)
	}
	return response.Result, nil
}
