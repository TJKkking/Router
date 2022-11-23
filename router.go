package main

import (
	"bufio"
	"bytes"
	"encoding/gob"
	"fmt"
	"log"
	"net"
	"os"
	"strconv"
	"strings"
	"time"
)

const (
	IPv4_address = "127.0.0.1"
	TTL          = 32
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

// point to router instance created by user
var routers map[string]*Router

func (r *Router) Listen() {
	lsAddr := &net.UDPAddr{IP: net.IP(IPv4_address), Port: r.Port}
	conn, err := net.ListenUDP("udp", lsAddr)
	checkError(err)
	defer conn.Close()

	for {
		buf := make([]byte, 4096)
		_, srcAddr, err := conn.ReadFromUDP(buf)
		checkError(err)
		log.Printf("[receive]table from port: %d\n", srcAddr.Port)

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
					if !r.refuseCheck(v.Route) { // 不包含refused node 更新
						tobeUpdate = true
						log.Println("[update][router-" + r.ID + ", from-" + srcRouter.ID + ", des-" + k + "]node already existed, same next hop")
					}

				} else if r.RoutingTable[k].Distance > v.Distance+1 { // 下一跳不同且path更短
					if !r.refuseCheck(v.Route) { // 不包含refused node 更新
						tobeUpdate = true
						log.Println("[update][router-" + r.ID + ", from-" + srcRouter.ID + ", des-" + k + "]node already existed, different next hop")
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
				if !r.refuseCheck(v.Route) { // 新节点且path不包含refused node
					r.RoutingTable[k] = TableEntry{
						Distance: v.Distance + 1,
						Refused:  false,
						Route:    append([]string{srcRouter.ID}, v.Route...),
					}
					log.Printf("[update][router-" + r.ID + ", from-" + srcRouter.ID + ", des-" + k + "]new node\n")
				}
			}
		}
		// todo: save update INFO to log
		// todo: loop?
	}
}

func (r *Router) refuseCheck(route []string) bool {
	var isRefused bool = false
	for _, y := range route {
		if r.RoutingTable[y].Refused == true { //Refused
			isRefused = true
			break
		}
	}
	return isRefused
}

func (r *Router) Broadcast() {
	for {
		for _, port := range r.Neighbors {
			address := IPv4_address + ":" + strconv.Itoa(port)
			dstAddr, err := net.ResolveUDPAddr("udp", address)
			checkError(err)

			err = sendRoutingTable(dstAddr, r)
			checkError(err)
		}
		log.Printf("[Broadcast]Router %s send a update\n", r.ID)
		time.Sleep(LOOP * time.Second)
	}
}

func sendRoutingTable(dstAddr *net.UDPAddr, r *Router) error { //todo r *Router
	//srcAddr := &net.UDPAddr{IP: net.IP(IPv4_address), Port: r.Port}
	conn, err := net.DialUDP("udp", nil, dstAddr) // use random port to send data
	checkError(err)
	defer conn.Close()

	var network bytes.Buffer
	enc := gob.NewEncoder(&network)
	gob.Register(r) //Regist type
	err = enc.Encode(r)
	checkError(err)

	n, err := conn.Write(network.Bytes())
	defer network.Reset()
	fmt.Printf("Router "+r.ID+"send %d bytes data to Port: %d\n", n, dstAddr.Port)
	return err
}

func (r *Router) printTable() {
	for k, v := range r.RoutingTable {
		if r.ID == k {
			continue
		}
		fmt.Print("     ", k, "        ", v.Distance, "   ")
		for i := 0; i < len(v.Route); i++ {
			fmt.Print(v.Route[i])
			if i < len(v.Route)-1 {
				fmt.Print(",")
			}
		}
		fmt.Println()
	}
}

func (r *Router) Daemon() {

}

func checkError(err error) {
	if err != nil {
		fmt.Println("[ERROR]Some error occurred: " + err.Error())
		return
	}
}

/*
* input
* 0 "router"
* 1 id
* 2 myPort
* 3 nbPort []string
 */
func initRouter(input []string) error { // nbs contain nbs' port
	cur := input[1]
	if routers[cur] != nil {
		//log.Println("[ERROE]init faild, router " + cur + "already exist")
		return fmt.Errorf("[ERROE]init faild, router " + cur + "already exist")
	}

	var nbs []int
	port, _ := strconv.Atoi(input[2])
	for _, v := range input[3:] {
		nbport, _ := strconv.Atoi(v)
		nbs = append(nbs, nbport)
	}
	routers[cur] = NewRouter(input[1], port, nbs)

	fmt.Println("NEW ROUTER:")
	fmt.Println("id: " + routers[cur].ID)
	fmt.Printf("port: %d\n", routers[cur].Port)
	fmt.Print("Neighbors: ")
	fmt.Println(routers[cur].Neighbors)

	//加入指向自己的路由项to broadcast
	routers[cur].RoutingTable[cur] = TableEntry{
		Refused:  false,
		Route:    []string{},
		Distance: 0,
	}

	go routers[cur].Listen()
	go routers[cur].Broadcast()
	fmt.Println("[INFO]init Router id: " + cur + ", port: " + input[2])

	return nil
}

func NewRouter(id string, port int, nbs []int) *Router {
	return &Router{
		ID:           id,
		Port:         port,
		Neighbors:    nbs,
		RoutingTable: make(map[string]TableEntry),
	}
}

func main() {
	fmt.Println("[INFO]The router execution environment is initialized successfully!")
	fmt.Println("[warm]NO ROUTER in the environment!")
	fmt.Println("[INFO]You can use command [router ID myport port1 port2 port3…] to create a router")
	fmt.Println("[INFO]Example: router 4 3004 3003 3006 3005")

	// 分配内存空间，nil map 不能赋值
	routers = make(map[string]*Router)

	var curRouter string = "none"
	for {
		fmt.Println("\nCurrent Router: " + curRouter)
		fmt.Print("> ")
		reader := bufio.NewReader(os.Stdin)
		input, err := reader.ReadString('\n')
		checkError(err)

		input = strings.TrimSpace(input)
		command := strings.Split(input, " ")

		switch command[0] {
		case "router": //create a new router
			err := initRouter(command)
			checkError(err)
			curRouter = command[1]
		case "RT": // print routing table
			routers[curRouter].printTable()
		case "SW": // switch to another router
			//todo check des num
		}
	}
}
