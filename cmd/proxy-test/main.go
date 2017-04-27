package main

import (
	"flag"
	"fmt"
	"net"
)

func main() {
	var (
		listenAddr  = flag.String("listen", "", "address to listen on")
		connectAddr = flag.String("connect", "", "address to listen to")
	)

	flag.Parse()

	l, err := net.Listen("tcp", *listenAddr)
	if err != nil {
		panic(err)
	}

	for {
		c, err := l.Accept()
		if err != nil {
			panic(err)
		}

		fmt.Printf("got a connection from %s\n", c.RemoteAddr())

		go connectToRemote(*connectAddr, c)
	}
}

func connectToRemote(addr string, uc net.Conn) {
	dc, err := net.Dial("tcp", addr)
	if err != nil {
		fmt.Println("error dialing ", addr)
		panic(err)
	}

	fmt.Printf("conn for %s should be open, trying this thing\n", addr)
	errs := make(chan error)
	go proxy(dc, uc, errs)
	go proxy(uc, dc, errs)

	fmt.Println(<-errs)

}

func proxy(reader net.Conn, writer net.Conn, errch chan<- error) {

	m := make([]byte, 4096)

	pathStr := fmt.Sprintf("%s->%s", reader.RemoteAddr(), writer.RemoteAddr())
	for {
		nr, err := reader.Read(m)
		fmt.Printf("%s: read %d bytes\n", pathStr, nr)
		if err != nil {
			fmt.Println(pathStr, ": error from read: ", err)
			errch <- err
			return
		}

		w := m[:nr]

		_, err = writer.Write(w)
		if err != nil {
			fmt.Println(pathStr, ": error writing: ", err)
		}

		for i := 0; i < 4096; i++ {
			m[i] = 0
		}
	}
}
