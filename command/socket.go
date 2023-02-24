package command

import (
	"log"

	zmq "github.com/pebbe/zmq4"
)

func NewWorkerSocket() (*zmq.Socket, error) {
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
	return sck, err
}
