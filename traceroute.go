package main 

import (
	"errors"
	"fmt"
	"net"
	"syscall"
	"time"
	"flag"
)

const DEFAULT_PORT = 33434
const DEFAULT_MAX_HOPS = 64
const DEFAULT_TIMEOUT_MS = 500
const DEFAULT_RETRIES = 3
const DEFAULT_PACKET_SIZE = 52

// TracrouteOptions type
type TracerouteOptions struct {
	port       int
	maxHops    int
	timeoutMs  int
	retries    int
	packetSize int
}


// TracerouteHop type
type TracerouteHop struct {
	Success     bool
	Address     [4]byte
	Host        string
	N           int
	ElapsedTime time.Duration
	TTL         int
}

// TracerouteResult type
type TracerouteResult struct {
	DestinationAddress [4]byte
	Hops               []TracerouteHop
}


// Return the first non-loopback address as a 4 byte IP address. This address
// is used for sending packets out.
func socketAddr() (addr [4]byte, err error) {
	addrs, err := net.InterfaceAddrs()
	if err != nil {
		return
	}

	for _, a := range addrs {
		if ipnet, ok := a.(*net.IPNet); ok && !ipnet.IP.IsLoopback() {
			if len(ipnet.IP.To4()) == net.IPv4len {
				copy(addr[:], ipnet.IP.To4())
				return
			}
		}
	}
	err = errors.New("You do not appear to be connected to the Internet")
	return
}

func destAddr(dest string)(destAddr [4]byte,err error){
	addrs,err := net.LookupHost(dest)
	if err != nil{
		return
	}
	addr := addrs[0]
	
	ipAddr,err := net.ResolveIPAddr("ip",addr)
	if err != nil{
		return
	}	
	copy(destAddr[:],ipAddr.IP.To4())
	return

}
func (options *TracerouteOptions) Port() int{
	if options.port == 0{
		options.port = DEFAULT_PORT
	}
	return options.port
}

func (options *TracerouteOptions) SetPort(port int){
	options.port = port
}
func (options *TracerouteOptions) MaxHops()int{
	if options.maxHops == 0{
		options.maxHops = DEFAULT_MAX_HOPS
	}
	return options.maxHops
}


func (options *TracerouteOptions) SetMaxHops(maxHops int){
	options.maxHops = maxHops
}
func (options *TracerouteOptions) TimeoutMs()int{
	if options.timeoutMs == 0{
		options.timeoutMs = DEFAULT_TIMEOUT_MS 
	}

	return options.timeoutMs
}


func (options *TracerouteOptions) SetTimeoutMs(tm int){
	options.timeoutMs = tm 
}

func (options *TracerouteOptions) Retries()int{
	if options.retries == 0{
		options.retries = DEFAULT_RETRIES
	}

	return options.retries
}


func (options *TracerouteOptions) SetRetries(retries int){
	options.retries = retries
}

func (options *TracerouteOptions) PacketSize()int{
	if options.packetSize == 0{
		options.packetSize = DEFAULT_PACKET_SIZE
	}

	return options.packetSize
}


func (options *TracerouteOptions) SetPacketSize(size int){
	options.packetSize =  size
}

func (hop *TracerouteHop )AddressString() string{
	return fmt.Sprintf("%v.%v.%v.%v",hop.Address[0],hop.Address[1],hop.Address[2],hop.Address[3])	
}

func (hop *TracerouteHop ) HostOrAddressString() string{
	hostOrAddr := hop.AddressString()
	if hop.Host != "" {
		hostOrAddr = hop.Host	
	}
	return hostOrAddr 
}

func notify(hop TracerouteHop,channels []chan TracerouteHop){
	for _ ,c := range channels{
		c <- hop
	}
}
func closeNotify(channels []chan TracerouteHop){
	for _,c := range channels{
		close(c)
	}
}
func Traceroute(dest string, options *TracerouteOptions, c ...chan TracerouteHop) (result TracerouteResult, err error) {
	result.Hops = []TracerouteHop{}
	destAddr,err := destAddr(dest)
	result.DestinationAddress = destAddr
	sockAddr ,err := socketAddr()
	if err != nil{
		return
	}
	
	timeoutMs := (int64)(options.TimeoutMs())
	tv := syscall.NsecToTimeval(1000 * 1000 * timeoutMs)
	
	ttl := 1
	retry := 0
	for {
		start := time.Now()
		recvSock,err := syscall.Socket(syscall.AF_INET,syscall.SOCK_RAW,syscall.IPPROTO_ICMP)
		if err != nil{
			return result,err
		}
		sendSock,err := syscall.Socket(syscall.AF_INET,syscall.SOCK_DGRAM,syscall.IPPROTO_UDP)
		if err != nil{
			return result,err
		}
		syscall.SetsockoptInt(sendSock,0x0,syscall.IP_TTL,ttl)
		
		syscall.SetsockoptTimeval(recvSock,syscall.SOL_SOCKET,syscall.SO_RCVTIMEO,&tv)
		defer syscall.Close(sendSock)	
		defer syscall.Close(recvSock)	
		
		syscall.Bind(recvSock,&syscall.SockaddrInet4{Port:options.Port(),Addr:sockAddr})
		
		syscall.Sendto(sendSock,[]byte{0x0},0,&syscall.SockaddrInet4{Port:options.Port(),Addr:destAddr})

		var p = make([]byte,options.PacketSize())
		n,form,err := syscall.Recvfrom(recvSock,p,0)
		elapsed := time.Since(start)
		if err == nil{
			currAddr := form.(*syscall.SockaddrInet4).Addr
			hop := TracerouteHop{Success:true,Address:currAddr,N:n,ElapsedTime:elapsed,TTL:ttl}
			currHost,err := net.LookupAddr(hop.AddressString())
			if err == nil{
				hop.Host = currHost[0]
			}
			notify(hop,c)	
			
			result.Hops = append(result.Hops,hop)
			ttl += 1
			retry = 0
			
			if ttl > options.MaxHops() || currAddr == destAddr{
				closeNotify(c)
				return result , nil
			}
			
		}else{
			retry += 1
			if retry > options.Retries(){
				notify(TracerouteHop{Success:false,TTL:ttl},c)
				ttl += 1
				retry = 0
			}
		}

	}
	return
}


func printHop(hop TracerouteHop) {
	addr := fmt.Sprintf("%v.%v.%v.%v", hop.Address[0], hop.Address[1], hop.Address[2], hop.Address[3])
	hostOrAddr := addr
	if hop.Host != "" {
		hostOrAddr = hop.Host
	}
	if hop.Success {
		fmt.Printf("%-3d %v (%v)  %v\n", hop.TTL, hostOrAddr, addr, hop.ElapsedTime)
	} else {
		fmt.Printf("%-3d *\n", hop.TTL)
	}
}

func address(address [4]byte) string {
	return fmt.Sprintf("%v.%v.%v.%v", address[0], address[1], address[2], address[3])
}



func main(){
	var m = flag.Int("m", DEFAULT_MAX_HOPS, `Set the max time-to-live (max number of hops) used in outgoing probe packets (default is 64)`)
	var q = flag.Int("q", 1, `Set the number of probes per "ttl" to nqueries (default is one probe).`)

	flag.Parse()
	host := flag.Arg(0)
	options := TracerouteOptions{}
	options.SetRetries(*q - 1)
	options.SetMaxHops(*m + 1)

	ipAddr, err := net.ResolveIPAddr("ip", host)
	if err != nil {
		return
	}

	fmt.Printf("traceroute to %v (%v), %v hops max, %v byte packets\n", host, ipAddr, options.MaxHops(), options.PacketSize())
	c := make(chan TracerouteHop, 0)
	go func() {
		for {
			hop, ok := <-c
			if !ok {
				fmt.Println()
				return
			}
			printHop(hop)
		}
	}()

	_, err = Traceroute(host, &options, c)
	if err != nil {
		fmt.Printf("Error: ", err)
	}
	
}
