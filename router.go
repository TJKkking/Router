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
	MAX_HOP      = 16
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

type DataPacket struct {
	Source      string
	Destination string
	TTL         int
	Payload     string
}

// point to router instance created by user
var routers map[string]*Router

func (r *Router) Listen() {
	lsAddr := &net.UDPAddr{IP: net.ParseIP(IPv4_address), Port: r.Port}
	conn, err := net.ListenUDP("udp", lsAddr)
	checkError("net.ListenUDP", err)
	defer conn.Close()

	for {
		buf := make([]byte, 4096)
		_, _, err := conn.ReadFromUDP(buf)
		checkError("ReadFromUDP", err)

		data := bytes.NewBuffer(buf)
		decoder := gob.NewDecoder(data)
		var srcRouter Router
		err = decoder.Decode(&srcRouter) //input must be memory address
		checkError("Decode", err)
		log.Println("[Receive]Router " + r.ID + " receive table from [Router " + srcRouter.ID + "]")

		srcTable := srcRouter.RoutingTable
		for k, v := range srcTable {
			if r.RoutingTable[k].Refused { // destination is refused todo
				continue
			}
			if _, ok := r.RoutingTable[k]; ok { //already existed
				var tobeUpdate bool = false
				// 排除自路由项&邻居的自路由项
				if (r.ID != k) && len(v.Route) > 0 && (r.RoutingTable[k].Route[0] == v.Route[0]) { // 下一跳相同
					if !r.refuseCheck(v.Route) { // 不包含refused node 更新
						tobeUpdate = true
						log.Println("[Update][router-" + r.ID + ", from-" + srcRouter.ID + ", des-" + k + "]node already existed, same next hop")
					}

				} else if r.RoutingTable[k].Distance > v.Distance+1 { // 下一跳不同且path更短
					if !r.refuseCheck(v.Route) { // 不包含refused node 更新
						tobeUpdate = true
						log.Println("[Update][router-" + r.ID + ", from-" + srcRouter.ID + ", des-" + k + "]node already existed, different next hop")
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
					log.Println("[Update][router " + r.ID + ", des " + k + " from router " + srcRouter.ID + "" + "]new node")
				}
			}
		}
		// todo: save update INFO to log
		// todo: loop?
	}
}

func (r *Router) receivePacket() {
	lsAddr := &net.UDPAddr{IP: net.ParseIP(IPv4_address), Port: r.Port + 1000} // 1000 offset
	conn, err := net.ListenUDP("udp", lsAddr)
	checkError("net.ListenUDP", err)
	defer conn.Close()

	for {
		buf := make([]byte, 4096)
		_, _, err := conn.ReadFromUDP(buf)
		checkError("ReadFromUDP", err)

		data := bytes.NewBuffer(buf)
		decoder := gob.NewDecoder(data)
		var packet DataPacket
		err = decoder.Decode(&packet) //input must be memory address
		checkError("Decode", err)
		log.Println("[DataPacket]Router " + r.ID + " receive DataPacket")

		dis := r.RoutingTable[packet.Destination].Distance
		// no neighbor
		if len(r.Neighbors) == 0 {
			fmt.Println("Router ", r.ID, ": No route to ", packet.Destination)
			continue
		}
		// no route
		if _, ok := r.RoutingTable[packet.Destination]; !ok {
			fmt.Println("Router ", r.ID, ": Dropped ", packet.Destination)
			continue
		}
		// unreachable
		if dis > MAX_HOP {
			fmt.Println("Router ", r.ID, ": Dropped ", packet.Destination)
			continue
		}
		// refused
		route := r.RoutingTable[packet.Destination].Route
		if r.refuseCheck(route) {
			fmt.Println("Router ", r.ID, ": Refuse route to ", packet.Destination)
			continue
		}

		// destination
		if packet.Destination == r.ID { // arrive
			fmt.Println("Router ", r.ID, ": Destination ", packet.Destination)
		} else if packet.TTL == 0 { // ttl = 0
			fmt.Println("Router ", r.ID, ": Time exceeded ", packet.Destination)
		} else if dis == 1 { // neighbor is des
			err := r.sendPacket(packet.Source, packet.Destination, packet.TTL-1, packet.Payload)
			checkError("sendPacket neighbor", err)
			fmt.Println("Router ", r.ID, ": Direct to ", packet.Destination)
		} else if dis > 1 && dis < MAX_HOP { // forward
			err := r.sendPacket(packet.Source, packet.Destination, packet.TTL-1, packet.Payload)
			checkError("sendPacket forward", err)
			fmt.Println("Router ", r.ID, ": Forward to ", packet.Destination)
		}
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
			checkError("ResolveUDPAddr", err)

			err = r.sendRoutingTable(dstAddr)
			checkError("sendRoutingTable", err)
		}
		log.Printf("[Broadcast]Router %s send a update\n", r.ID)
		time.Sleep(LOOP * time.Second)
	}
}

func (r *Router) sendRoutingTable(dstAddr *net.UDPAddr) error { //todo r *Router, done, tobe test
	//srcAddr := &net.UDPAddr{IP: net.IP(IPv4_address), Port: r.Port}
	conn, err := net.DialUDP("udp", nil, dstAddr) // use random port to send data
	checkError("DialUDP", err)
	defer conn.Close()

	var network bytes.Buffer
	enc := gob.NewEncoder(&network)
	gob.Register(r) //Regist type
	err = enc.Encode(r)
	checkError("Encode", err)

	// 返回字节数n省略
	_, err = conn.Write(network.Bytes())
	defer network.Reset()
	//fmt.Printf("[Router "+r.ID+"] send %d bytes data to Port: %d\n", n, dstAddr.Port)
	return err
}

func (r *Router) printTable() {
	fmt.Println("     Destination  Distance  Route")
	for k, v := range r.RoutingTable {
		if r.ID == k {
			continue
		}
		fmt.Print("     ", k, "            ", v.Distance, "          ")
		for i := 0; i < len(v.Route); i++ {
			fmt.Print(v.Route[i])
			if i < len(v.Route)-1 {
				fmt.Print(",")
			}
		}
		fmt.Println()
	}
}

func (r *Router) sendPacket(src string, des string, ttl int, payload string) error { // reassembly & sending
	//todo, done, to be test
	address := IPv4_address + ":" + strconv.Itoa(routers[des].Port)
	dstAddr, err := net.ResolveUDPAddr("udp", address)
	checkError("ResolveUDPAddr", err)

	conn, err := net.DialUDP("udp", nil, dstAddr)
	checkError("DialUDP", err)
	defer conn.Close()

	//reassembly
	pack := &DataPacket{
		Source:      src,
		Destination: des,
		TTL:         ttl,
		Payload:     payload,
	}
	var network bytes.Buffer
	enc := gob.NewEncoder(&network)
	gob.Register(pack) //Regist type
	err = enc.Encode(pack)
	checkError("Encode", err)
	_, err = conn.Write(network.Bytes())
	defer network.Reset()

	return err
}

func checkError(head string, err error) {
	if err != nil {
		fmt.Println("[ERROR]Error in " + head + ": " + err.Error())
		return
	}
}

/*
* input
* 0 "router"
* 1 id
* 2 myPort
* 3 nbPorts []string
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

func CheckSwitch(command []string) error {
	// Missing parameter
	if len(command) < 2 {
		return fmt.Errorf("Missing parameter: [ID]")
	}

	// Too many parameters
	if len(command) > 2 {
		return fmt.Errorf("Too many parameters!")
	}

	// invalid parameter
	if routers[command[1]] == nil {
		return fmt.Errorf("Invalid parameter: [ID]")
	}

	return nil
}

func (r *Router) PrintAdj() {
	var isEmpty bool = true
	for k, v := range r.RoutingTable {
		if v.Distance == 1 {
			isEmpty = false
			fmt.Print(k, " ")
		}
	}
	if isEmpty {
		fmt.Print("Empty")
	}
	fmt.Print("\n")
}

func (r *Router) RefuseNode(input []string) error {
	if len(input) != 2 {
		return fmt.Errorf("Invalid input!")
	}
	if _, ok := r.RoutingTable[input[1]]; ok {
		r.RoutingTable[input[1]] = TableEntry{
			Refused:  true,
			Distance: r.RoutingTable[input[1]].Distance, // 可达
			Route:    r.RoutingTable[input[1]].Route,
		}
	} else {
		r.RoutingTable[input[1]] = TableEntry{ // not exist
			Refused:  true,
			Distance: MAX_HOP,
			Route:    []string{},
		}
	}
	return nil
}

func statistics() {
	//num of routers
	//router list (id, port, nbs)
	//detail  of each router(receive, broadcast, update, table-wide)

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
		checkError("ReadString", err)

		input = strings.TrimSpace(input)
		command := strings.Split(input, " ")

		switch command[0] {
		case "router": //create a new router
			// todo validating parameters[ID, port] 2000<port<3000
			err := initRouter(command)
			checkError("initRouter", err)
			if curRouter == "none" {
				curRouter = command[1]
			}

		case "RT": // print routing table
			if curRouter == "none" {
				fmt.Println("[ERROR] No router has been created yet.")
				continue
			}
			routers[curRouter].printTable()

		case "SW": // switch to another router
			err = CheckSwitch(command)
			if err != nil {
				fmt.Printf("[ERROR] %v\n", err)
				fmt.Println("[Usage] SW [ID]    example: SW 2")
				continue
			}
			curRouter = command[1]

		case "N": // Print activity’s adjacent list.
			// to be test
			routers[curRouter].PrintAdj()

		case "D":
			//todo
			//optional ttl
		case "P":
			//todo
		case "R":
			//todo check
			routers[curRouter].RefuseNode(command)
		case "S":
			//todo
			statistics()
		}
	}
}
