package main

import (
	"bytes"
	"encoding/gob"
	"fmt"
	"net"
	"strconv"
	"time"
)

const (
	IPv4_address = "127.0.0.1"
	TTL          = 64
	LOOP         = 30
)

type Router struct {
	ID           string
	Port         int
	Neighbors    []int
	RoutingTable map[string]TableEntry
}

type TableEntry struct {
	Refused  bool
	Route    []int
	Distance int
}

func (r *Router) routerInit(id string, port int, nbs []int) {
	r.ID = id
	r.Port = port
	r.Neighbors = nbs
	r.RoutingTable = make(map[string]TableEntry)

}

func (r *Router) listen() {
	lsAddr := &net.UDPAddr{IP: net.IP(IPv4_address), Port: r.Port}
	conn, err := net.ListenUDP("udp", lsAddr)
	checkError(err)
	defer conn.Close()

	for {
		buf := make([]byte, 4096)
		_, srcAddr, err := conn.ReadFromUDP(buf)
		checkError(err)

		data := bytes.NewBuffer(buf)
		decoder := gob.NewDecoder(data)
		var srcRouter Router
		err = decoder.Decode(&srcRouter) //input must be memory address
		checkError(err)

		srcTable = srcRouter.RoutingTable

	}
}

func (r *Router) broadcast() {
	for {
		for _, port := range r.Neighbors {
			address := IPv4_address + ":" + strconv.Itoa(port)
			dstAddr, err := net.ResolveUDPAddr("udp", address)
			checkError(err)

			err = sendRoutingTable(dstAddr, r)
			checkError(err)
		}
		time.Sleep(LOOP * time.Second)
	}
}

func sendRoutingTable(dstAddr *net.UDPAddr, r *Router) error { //todo r *Router
	srcAddr := &net.UDPAddr{IP: net.IP(IPv4_address), Port: r.Port}
	conn, err := net.DialUDP("udp", srcAddr, dstAddr)
	checkError(err)
	defer conn.Close()

	var network bytes.Buffer
	enc := gob.NewEncoder(&network)
	gob.Register(r) //Regist type
	err = enc.Encode(r)
	checkError(err)

	n, err := conn.Write(network.Bytes())
	defer network.Reset()
	fmt.Printf("Router "+r.ID+"send %d bytes data to Port: %d", n, dstAddr.Port)
	return err
}

func (r *Router) daemon() {

}

func checkError(err error) {
	if err != nil {
		fmt.Printf("Some error occurred: " + err.Error())
		return
	}
}
