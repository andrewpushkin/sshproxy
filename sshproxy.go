package sshproxy

import (
	"io"
	"log"
	"net"

	"golang.org/x/crypto/ssh"
)

type SshConn struct {
	net.Conn
	config     *ssh.ServerConfig
	callbackFn func(c ssh.ConnMetadata) (*ssh.Client, error)
	wrapFn     func(c ssh.ConnMetadata, r io.ReadCloser) (io.ReadCloser, error)
	closeFn    func(c ssh.ConnMetadata) error
}

func (p *SshConn) serve() error {
	serverConn, chans, reqs, err := ssh.NewServerConn(p, p.config)
	if err != nil {
		log.Println("failed to handshake")
		return (err)
	}
	log.Println("wwwwwwwwwwwwwwwww")
	defer serverConn.Close()

	clientConn, err := p.callbackFn(serverConn)
	if err != nil {
		log.Printf("%s", err.Error())
		return (err)
	}

	defer clientConn.Close()

	go ssh.DiscardRequests(reqs)

	for newChannel := range chans {

		channel_client, requests_client, err := clientConn.OpenChannel(newChannel.ChannelType(), newChannel.ExtraData())
		if err != nil {
			log.Printf("Could not accept client channel_server: %s", err.Error())
			return err
		}

		channel_server, requests_server, err := newChannel.Accept()
		if err != nil {
			log.Printf("Could not accept server channel_server: %s", err.Error())
			return err
		}

		// connect requests_server
		go func() {
			log.Printf("Waiting for request")

		r:
			for {
				var req *ssh.Request
				var dst ssh.Channel

				select {
				case req = <-requests_server:
					dst = channel_client
				case req = <-requests_client:
					dst = channel_server
				}

				//	log.Printf("Request: %s %s %s %s\n", dst, req.Type, req.WantReply, req.Payload)

				b, err := dst.SendRequest(req.Type, req.WantReply, req.Payload)
				if err != nil {
					log.Printf("%s", err)
				}

				if req.WantReply {
					req.Reply(b, nil)
				}

				log.Println("Req type:", req.Type)
				switch req.Type {
				case "exit-status":
					break r
				case "exec":
					// not supported (yet)
				default:
					log.Println(req.Type)
				}
			}

			channel_server.Close()
			channel_client.Close()
		}()

		// connect channels
		log.Printf("Connecting channels.")

		var wrappedChannel io.ReadCloser = channel_server
		var wrappedChannel2 io.ReadCloser = channel_client

		if p.wrapFn != nil {
			// wrappedChannel, err = p.wrapFn(channel_server)
			wrappedChannel2, err = p.wrapFn(serverConn, channel_client)
		}

		go io.Copy(channel_client, wrappedChannel)
		go io.Copy(channel_server, wrappedChannel2)

		defer wrappedChannel.Close()
		defer wrappedChannel2.Close()
	}

	if p.closeFn != nil {
		p.closeFn(serverConn)
	}

	return nil
}

func ListenAndServe(addr string, serverConfig *ssh.ServerConfig,
	callbackFn func(c ssh.ConnMetadata) (*ssh.Client, error),
	wrapFn func(c ssh.ConnMetadata, r io.ReadCloser) (io.ReadCloser, error),
	closeFn func(c ssh.ConnMetadata) error,
) error {
	listener, err := net.Listen("tcp", addr)
	if err != nil {
		log.Printf("net.Listen failed: %v", err)
		return err
	}

	defer listener.Close()

	for {
		conn, err := listener.Accept()
		if err != nil {
			log.Printf("listen.Accept failed: %v", err)
			return err
		}

		sshconn := &SshConn{Conn: conn, config: serverConfig, callbackFn: callbackFn, wrapFn: wrapFn, closeFn: closeFn}

		go func() {
			if err := sshconn.serve(); err != nil {
				log.Printf("Error occured while serving %s\n", err)
				return
			}

			log.Println("Connection closed.")
		}()
	}

}
