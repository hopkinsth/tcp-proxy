package main

import (
	"flag"
	"fmt"
	"io/ioutil"
	"net"
	"os"
	"os/user"
	"strings"
	"time"

	"golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/knownhosts"
)

func main() {
	var (
		listenAddr  = flag.String("listen", "", "address to listen on")
		connectAddr = flag.String("connect", "", "address to connect to")
	)

	flag.Parse()
	if *listenAddr == "" || *connectAddr == "" {
		fmt.Println("missing addresses")
		flag.Usage()
		return
	}

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
	dc, err := getConn(addr)
	if err != nil {
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

// getConn returns a tcp or ssh conn for now
func getConn(addr string) (net.Conn, error) {
	switch {
	default:
		fallthrough
	case strings.Index(addr, "tcp://") == 0:
		return net.DialTimeout("tcp", strings.Replace(addr, "tcp://", "", -1), time.Second*30)
	case strings.Index(addr, "ssh://") == 0:
		spec := strings.Replace(addr, "ssh://", "", -1)
		// split on the arrow
		parts := strings.SplitN(spec, "->", 2)
		if len(parts) != 2 {
			return nil, fmt.Errorf("invalid ssh connection spec; we are tunneling here")
		}

		sshHost := parts[0]
		tunHost := parts[1]

		var uname string
		var sysUser *user.User
		if hu := strings.Index(sshHost, "@"); hu != -1 {
			uname = sshHost[:hu]
			sshHost = sshHost[hu+1:]
		} else {
			u, err := user.Current()
			if err != nil {
				return nil, fmt.Errorf("error getting current user: %s", err)
			}
			uname = u.Username
		}

		var keyCheck ssh.HostKeyCallback
		if sysUser != nil {
			var err error
			keyCheck, err = knownhosts.New(fmt.Sprintf("%s/.ssh/known_hosts", sysUser.HomeDir))
			if err != nil {
				return nil, fmt.Errorf("error opening known hosts db: %s", err)
			}
		} else {
			keyCheck = ssh.InsecureIgnoreHostKey()
		}

		sc := &ssh.ClientConfig{
			User:            uname,
			HostKeyCallback: keyCheck,
			Timeout:         time.Second * 30,
			Auth:            []ssh.AuthMethod{ssh.PublicKeysCallback(getSSHKeys)},
		}

		fmt.Printf("connecting to %s@%s", uname, sshHost)
		sshCon, err := ssh.Dial("tcp", sshHost, sc)
		if err != nil {
			return nil, fmt.Errorf("error connecting to ssh host: %s", err)
		}

		fcon, err := sshCon.Dial("tcp", tunHost)
		if err != nil {
			return nil, fmt.Errorf("error connecting through tunnel: %s", err)
		}
		return fcon, nil
	}
}

func getSSHKeys() ([]ssh.Signer, error) {
	cu, err := user.Current()
	if err != nil {
		return nil, fmt.Errorf("error getting current user: %s", err)
	}

	sshDir := fmt.Sprintf("%s/.ssh", cu.HomeDir)
	dir, err := os.Open(sshDir)
	if err != nil {
		return nil, fmt.Errorf("error opening ssh dir: %s", err)
	}

	sshkeys, err := dir.Readdirnames(-1)
	if err != nil {
		return nil, fmt.Errorf("error reading directory: %s", err)
	}

	var signers []ssh.Signer
	for _, file := range sshkeys {
		if file == "config" || file == "known_hosts" || file == "authorized_keys" || strings.Index(file, "id_") != 0 {
			continue
		}

		b, err := ioutil.ReadFile(fmt.Sprintf("%s/%s", sshDir, file))
		if err != nil {
			fmt.Printf("error reading potential ssh key: %s\n", err)
			continue
		}

		s, err := ssh.ParsePrivateKey(b)
		if err != nil {
			fmt.Printf("error parsing potential ssh key %s/%s:\n\t%s\n", sshDir, file, err)
			continue
		}

		fmt.Printf("got private key %s/%s\n", sshDir, file)

		signers = append(signers, s)
	}

	return signers, nil

}
