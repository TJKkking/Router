package main

import (
	"bytes"
	"encoding/gob"
	"fmt"
	"log"
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
	Route    []string
	Distance int
}

func (r *Router) routerInit(id string, port int, nbs []int) {
	r.ID = id
	r.Port = port
	r.Neighbors = nbs
	r.RoutingTable = make(map[string]TableEntry)

	// for _, v := range r.Neighbors {
	// 	r.RoutingTable[strconv.Itoa(v)] = TableEntry{
	// 		Refused:  false,
	// 		Route:    []string{},
	// 		Distance: 1,
	// 	}
	// }
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
		log.Printf("[receive]table from port: %d", srcAddr.Port)

		data := bytes.NewBuffer(buf)
		decoder := gob.NewDecoder(data)
		var srcRouter Router
		err = decoder.Decode(&srcRouter) //input must be memory address
		checkError(err)

		srcTable := srcRouter.RoutingTable
		for k, v := range srcTable {
			if r.RoutingTable[k].Refused { // destination is refused todo
				continue
			}
			if _, ok := r.RoutingTable[k]; ok { //already existed
				var tobeUpdate bool = false
				if r.RoutingTable[k].Route[0] == v.Route[0] { // 下一跳相同
					if !r.RefuseCheck(v.Route) { // 不包含refused node 更新
						tobeUpdate = true
						log.Printf("[update][router-" + r.ID + ", from-" + srcRouter.ID + ", des-" + k + "]node already existed, same next hop")
					}

				} else if r.RoutingTable[k].Distance > v.Distance+1 { // 下一跳不同且path更短
					if !r.RefuseCheck(v.Route) { // 不包含refused node 更新
						tobeUpdate = true
						log.Printf("[update][router-" + r.ID + ", from-" + srcRouter.ID + ", des-" + k + "]node already existed, different next hop")
					}
				}
				if tobeUpdate { // update routing table
					r.RoutingTable[k] = TableEntry{ //todo: 不可达的情况是否需要特殊考虑
						Refused:  false,
						Distance: v.Distance + 1,
						Route:    append([]string{srcRouter.ID}, v.Route...),
					}
				}
			} else { // new table entry
				if !r.RefuseCheck(v.Route) { // 新节点且path不包含refused node
					r.RoutingTable[k] = TableEntry{
						Distance: v.Distance + 1,
						Refused:  false,
						Route:    append([]string{srcRouter.ID}, v.Route...),
					}
					log.Printf("[update][router-" + r.ID + ", from-" + srcRouter.ID + ", des-" + k + "]new node")
				}
			}
		}
		// todo: save update info to log
		// todo: loop?
	}
}

func (r *Router) RefuseCheck(route []string) bool {
	var isRefused bool = false
	for _, y := range route {
		if r.RoutingTable[y].Refused == true { //Refused
			isRefused = true
			break
		}
	}
	return isRefused
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
