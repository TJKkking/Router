package main

import (
	"bufio"
	"bytes"
	"encoding/gob"
	"fmt"
	"io"
	"log"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// TTL: Default TTL
// MAX_HOP: Maximum TTL
const (
	IPv4_address = "127.0.0.1"
	TTL          = 15
	MAX_HOP      = 16
	LOOP         = 30
)

type Router struct {
	ID           string
	Port         int
	Neighbors    []int
	RoutingTable map[string]TableEntry
	info         InfoSet
}

type TableEntry struct {
	Refused  bool
	Priority bool
	Route    []string
	Distance int
}

type DataPacket struct {
	Source      string
	Destination string
	TTL         int
	Payload     string
}

type InfoSet struct {
	BroadTimes   int
	UpdateTimes  int
	ForwardTimes int
}

// todo usage of checkError()
// point to router instance created by user
var routers map[string]*Router
var logName string

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
		log.Println("[Receive  ]Router " + r.ID + " receive table from [Router " + srcRouter.ID + "]")

		srcTable := srcRouter.RoutingTable
		for k, v := range srcTable {
			if _, ok := r.RoutingTable[k]; ok { //already existed
				if r.RoutingTable[k].Refused || r.RoutingTable[k].Priority { // destination is refused OR set priority route
					continue
				}
				var tobeUpdate bool = false
				// 排除自路由项&邻居的自路由项
				if (r.ID != k) && len(v.Route) > 0 && (r.RoutingTable[k].Route[0] == v.Route[0]) { // 下一跳相同
					if !r.refuseCheck(v.Route) { // 不包含refused node 更新
						tobeUpdate = true
						log.Println("[Update   ][router-" + r.ID + ", from-" + srcRouter.ID + ", des-" + k + "]node already existed, same next hop")
					}

				} else if r.RoutingTable[k].Distance > v.Distance+1 { // 下一跳不同且path更短
					if !r.refuseCheck(v.Route) { // 不包含refused node 更新
						tobeUpdate = true
						log.Println("[Update   ][router-" + r.ID + ", from-" + srcRouter.ID + ", des-" + k + "]node already existed, different next hop")
					}
				}
				if tobeUpdate { // update routing table
					r.RoutingTable[k] = TableEntry{ //todo: 不可达的情况是否需要特殊考虑
						Refused:  false,
						Priority: false,
						Distance: v.Distance + 1,
						Route:    append([]string{srcRouter.ID}, v.Route...),
					}
					r.info.UpdateTimes += 1
				}
			} else { // new table entry
				if !r.refuseCheck(v.Route) { // 新节点且path不包含refused node
					r.RoutingTable[k] = TableEntry{
						Distance: v.Distance + 1,
						Refused:  false,
						Priority: false,
						Route:    append([]string{srcRouter.ID}, v.Route...),
					}
					r.info.UpdateTimes += 1
					log.Println("[Update   ][router " + r.ID + ", des " + k + " from router " + srcRouter.ID + "" + "]new node")
				}
			}
		}
	}
}

func (r *Router) receivePacket() { // goroutine
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

		err = r.forward(packet)
		if err != nil {
			fmt.Println("[Faild] Router ", r.ID, ": ", err)
		}
	}
}

func (r *Router) forward(packet DataPacket) error {
	dis := r.RoutingTable[packet.Destination].Distance
	var err error = nil
	if packet.Destination == r.ID { // arrive
		fmt.Println("Router", r.ID, "Destination ", packet.Destination)
		return nil
	}

	if len(r.Neighbors) == 0 { // no neighbor
		return fmt.Errorf("No route to %s", packet.Destination)
	}
	if _, ok := r.RoutingTable[packet.Destination]; !ok { // no route
		return fmt.Errorf("Dropped %s", packet.Destination)
	}
	if dis > MAX_HOP { // unreachable
		return fmt.Errorf("Dropped %s", packet.Destination)
	}
	route := r.RoutingTable[packet.Destination].Route
	if r.refuseCheck(route) { // refused
		return fmt.Errorf("Refuse route to %s", packet.Destination)
	}
	if packet.TTL == 0 { // ttl = 0
		return fmt.Errorf("Time exceeded %s", packet.Destination)
	}

	if dis == 1 { // neighbor is des
		err = r.sendPacket(packet.Source, packet.Destination, packet.TTL-1, packet.Payload)
		if err != nil {
			return err
		}
		fmt.Println("Router", r.ID, ": Direct to ", packet.Destination)
		r.info.ForwardTimes += 1
	} else if dis > 1 && dis < MAX_HOP { // forward
		err = r.sendPacket(packet.Source, r.RoutingTable[packet.Destination].Route[0], packet.TTL-1, packet.Payload)
		if err != nil {
			return err
		}
		fmt.Println("Router", r.ID, ": Forward to ", packet.Destination)
		log.Println("[Forward  ]Router", r.ID, ": Forward to ", packet.Destination)
		r.info.ForwardTimes += 1
	}
	return err
}

func (r *Router) sendPacket(src string, des string, ttl int, payload string) error { // reassembly & sending
	address := IPv4_address + ":" + strconv.Itoa(routers[des].Port+1000)
	dstAddr, err := net.ResolveUDPAddr("udp", address)
	checkError("ResolveUDPAddr", err)

	conn, err := net.DialUDP("udp", nil, dstAddr)
	checkError("DialUDP", err)
	defer conn.Close()

	//reassembly
	pack := &DataPacket{
		Source:      src,
		Destination: payload,
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
		r.info.BroadTimes += 1
		time.Sleep(LOOP * time.Second)
	}
}

func (r *Router) sendRoutingTable(dstAddr *net.UDPAddr) error {
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

func (r *Router) printTable() { // todo route 不包含des
	fmt.Println("     Destination  Distance  Route")
	for k, v := range r.RoutingTable {
		if r.ID == k {
			continue
		}
		fmt.Print("     ", k, "            ", v.Distance, "         ")
		for i := 0; i < len(v.Route); i++ {
			fmt.Print(v.Route[i])
			if i < len(v.Route)-1 {
				fmt.Print(",")
			}
		}
		fmt.Println()
	}
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
		log.Println("[ERROE    ]init faild, router " + cur + "already exist")
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
		Priority: false,
		Route:    []string{},
		Distance: 0,
	}
	routers[cur].info.BroadTimes = 0
	routers[cur].info.ForwardTimes = 0
	routers[cur].info.UpdateTimes = 0

	go routers[cur].Listen()
	go routers[cur].Broadcast()
	go routers[cur].receivePacket()
	log.Println("[INFO]init Router id: " + cur + ", port: " + input[2])

	return nil
}

func NewRouter(id string, port int, nbs []int) *Router {
	return &Router{
		ID:           id,
		Port:         port,
		Neighbors:    nbs,
		RoutingTable: make(map[string]TableEntry),
		info:         InfoSet{},
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
			Priority: false,
			Distance: r.RoutingTable[input[1]].Distance, // 可达
			Route:    r.RoutingTable[input[1]].Route,
		}
	} else {
		r.RoutingTable[input[1]] = TableEntry{ // not exist
			Refused:  true,
			Priority: false,
			Distance: MAX_HOP,
			Route:    []string{},
		}
	}
	log.Println("[Refuse   ] Router " + r.ID + "refuse node " + input[1])
	return nil
}

func (r *Router) sendProcess(input []string) error {
	var ttl int = TTL
	if len(input) == 3 { // 用户提供自定义ttl
		v, _ := strconv.Atoi(input[2])
		if v >= 0 && v < MAX_HOP { // validate
			ttl = v
		}
	}

	packet := &DataPacket{
		Source:      r.ID,
		Destination: input[1],
		TTL:         ttl,
		Payload:     input[1],
	}
	err := r.forward(*packet)
	if err != nil {
		fmt.Println("[Faild] Router ", r.ID, ": ", err)
	}

	return err
}

func (r *Router) priority(input []string) error {
	curNode := r.ID
	desNode := input[len(input)-1]
	path := []string{}
	for _, v := range input[2:] { //n1 - nk
		if _, ok := routers[curNode].RoutingTable[v]; ok { // next node is reachable
			path = append(path, routers[curNode].RoutingTable[v].Route...)
			curNode = v
		} else {
			return fmt.Errorf("[Faild] Faild to specify priority route: Router " + curNode + " CAN NOT route to Router " + v)
		}
	}
	// set priority route
	r.RoutingTable[desNode] = TableEntry{
		Route:    path,
		Distance: len(path),
		Refused:  r.RoutingTable[desNode].Refused,
		Priority: true,
	}
	return nil
}

func statistics() {
	num := len(routers)
	fmt.Println("=======================statistics=======================")
	fmt.Println(" Total number of Routers:", num)
	fmt.Print("\n")
	fmt.Println("  ID    Port    Neighbors    RefuseNode    [Times of Broadcast]  [Times of Update]  [Times of Forward]")
	for _, v := range routers {
		//fmt.Printf("  %s     %d    %v            ", v.ID, v.Port, v.Neighbors)
		fmt.Printf("  %s     %d       ", v.ID, v.Port)
		nbs := "["
		var isEmpty bool = true
		for k, v := range v.RoutingTable {
			if v.Distance == 1 {
				isEmpty = false
				nbs += k + ", "
			}
		}
		if isEmpty {
			nbs += "Empty"
		}
		nbs += "]"

		for {
			if len(nbs) > 13 {
				break
			}
			nbs = nbs + " "
		}

		str := "["
		for x, y := range v.RoutingTable {
			if y.Refused {
				str = str + x + ", "
			}
		}
		str = str + "]"
		for {
			if len(str) > 14 {
				break
			}
			str = str + " "
		}
		fmt.Print(nbs)
		fmt.Print(str)
		fmt.Printf("%d                     %d                 %d\n", v.info.BroadTimes, v.info.UpdateTimes, v.info.ForwardTimes)
	}
}

func printLog(input []string) { // read log
	var readLog []string
	row := 20
	if len(input) == 2 {
		row, _ = strconv.Atoi(input[1])
	} else if len(input) > 2 {
		fmt.Println("[error]invalid input! Please check again.")
		return
	}

	f, err := os.OpenFile(logName, os.O_RDONLY, 0)
	if err != nil {
		fmt.Println("[Faild]Failed to open log file: " + logName)
		return
	}
	defer f.Close()

	buff := make([]byte, 0, 4096)
	char := make([]byte, 1)

	stat, _ := f.Stat()
	filesize := stat.Size()

	var cursor int64 = 0
	cnt := 0
	for {
		cursor -= 1
		_, _ = f.Seek(cursor, io.SeekEnd)
		_, err = f.Read(char)
		if err != nil {
			fmt.Println("[Faild]Failed to open log file: " + logName)
			return
		}

		if char[0] == '\n' {
			if len(buff) > 0 {
				revers(buff)
				// 读取到的行
				readLog = append(readLog, string(buff))
				cnt++
				if cnt == row {
					// 超过数量退出
					break
				}

			}
			buff = buff[:0]
		} else {
			buff = append(buff, char[0])
		}

		if cursor == -filesize {
			break
		}
	}

	for i := len(readLog) - 1; i >= 0; i-- {
		fmt.Println(readLog[i])
	}
	return
}

func revers(s []byte) {
	for i, j := 0, len(s)-1; i < j; i, j = i+1, j-1 {
		s[i], s[j] = s[j], s[i]
	}
}

func GetAppPath() string {
	file, _ := exec.LookPath(os.Args[0])
	path, _ := filepath.Abs(file)
	index := strings.LastIndex(path, string(os.PathSeparator))

	return path[:index]
}

func main() {
	fmt.Println("[INFO]The router execution environment is initialized successfully!")
	fmt.Println("[WARM]NO ROUTER in the environment!")
	fmt.Println("[INFO]You can use command [router ID myport port1 port2 port3…] to create a router")
	fmt.Println("[INFO]Example: router 4 3004 3003 3006 3005")

	timeStr := time.Now().Format("2006-01-02_15:04:05")
	logName = "/Log/" + timeStr + ".txt"
	logName = GetAppPath() + logName
	logFile, err := os.OpenFile(logName, os.O_RDWR|os.O_CREATE|os.O_APPEND, 0766)
	if err != nil {
		panic(err)
	}
	log.SetOutput(logFile)

	// 分配内存空间，nil map 不能赋值
	routers = make(map[string]*Router)

	var curRouter string = "none"
	for {
		fmt.Println("\nCurrent Router: " + curRouter)
		//fmt.Print("> ")
		reader := bufio.NewReader(os.Stdin)
		input, err := reader.ReadString('\n')
		checkError("ReadString", err)

		input = strings.TrimSpace(input)
		command := strings.Split(input, " ")

		switch command[0] {
		case "router": //create a new router
			// todo validating 2000<port<3000
			err := initRouter(command)
			if err != nil {
				fmt.Println(err)
				continue
			}
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
			routers[curRouter].PrintAdj()
		case "D":
			routers[curRouter].sendProcess(command)
		case "P":
			err = routers[curRouter].priority(command)
			if err != nil {
				fmt.Println(err)
				continue
			}
		case "R":
			routers[curRouter].RefuseNode(command)
		case "S":
			statistics()
		case "L":
			printLog(command)
		case "exit":
			return
		}
	}
}
