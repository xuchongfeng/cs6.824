package viewservice

import "net"
import "net/rpc"
import "log"
import "time"
import "sync"
import "fmt"
import "os"
import "sync/atomic"


type ViewServer struct {
	mu       sync.Mutex
	l        net.Listener
	dead     int32 // for testing
	rpccount int32 // for testing
	me       string


	// Your declarations here.
    view View
    primaryAck uint
    primaryTick uint
    backupAck uint
    backupTick uint
    currentTick uint
}

func initView(view *View) {
    view.Viewnum = 0
    view.Primary = ""
    view.Backup = ""
}

// primary exist or not
func (vs *ViewServer) HasPrimary() bool {
    return vs.view.Primary != ""
}

func (vs *ViewServer) IsPrimary(server string) bool {
    return vs.view.Primary == server
}

// backup exist or not
func (vs *ViewServer) HasBackup() bool {
    return vs.view.Backup != ""
}

func (vs *ViewServer) IsBackup(server string) bool {
    return vs.view.Backup == server
}

// view acked by primary or not
func (vs *ViewServer) Acked() bool {
    return  vs.view.Viewnum != 0 && vs.view.Viewnum == vs.primaryAck
}

// promote backup to primary
func (vs *ViewServer) PromoteBackup() {
    if !vs.HasBackup() {
        return
    }
    vs.view.Primary = vs.view.Backup
    vs.view.Backup = ""
    vs.view.Viewnum++
    vs.primaryAck = vs.backupAck
    vs.primaryTick = vs.backupTick
}

//
// server Ping RPC handler.
//
func (vs *ViewServer) Ping(args *PingArgs, reply *PingReply) error {
	// Your code here.
    server := args.Me
    viewNum := args.Viewnum
    vs.mu.Lock()
    switch {
    // init state
    case !vs.HasPrimary() && vs.view.Viewnum == 0:
        vs.view.Primary = server
        vs.view.Viewnum = viewNum + 1
        vs.primaryTick = vs.currentTick
        vs.primaryAck = 0
    case vs.IsPrimary(server):
        if viewNum == 0 {
            vs.PromoteBackup()
        } else {
            vs.primaryTick = vs.currentTick
            vs.primaryAck = viewNum
        }
    case !vs.HasBackup() && vs.Acked():
        vs.view.Backup = server
        vs.view.Viewnum++
        vs.backupTick = vs.currentTick
    case vs.IsBackup(server):
        if viewNum == 0 && vs.Acked() {
            vs.view.Viewnum++
            vs.view.Backup = server
            vs.backupTick = vs.currentTick
        } else {
            vs.backupTick = vs.currentTick
        }
    }
    reply.View = vs.view
    vs.mu.Unlock()
    return nil
}

//
// server Get() RPC handler.
//
func (vs *ViewServer) Get(args *GetArgs, reply *GetReply) error {
	// Your code here.
    vs.mu.Lock()
    reply.View = vs.view
    vs.mu.Unlock()

	return nil
}


//
// tick() is called once per PingInterval; it should notice
// if servers have died or recovered, and change the view
// accordingly.
//
func (vs *ViewServer) tick() {
	// Your code here.
    // Check primary server
    vs.mu.Lock()
    vs.currentTick++

    if vs.Acked() && vs.HasBackup() && vs.currentTick - vs.backupTick >= DeadPings {
        vs.view.Backup = ""
        vs.view.Viewnum++
    }

    if vs.Acked() && vs.currentTick - vs.primaryTick >= DeadPings {
        vs.PromoteBackup()
    }

    vs.mu.Unlock()
}

//
// tell the server to shut itself down.
// for testing.
// please don't change these two functions.
//
func (vs *ViewServer) Kill() {
	atomic.StoreInt32(&vs.dead, 1)
	vs.l.Close()
}

//
// has this server been asked to shut down?
//
func (vs *ViewServer) isdead() bool {
	return atomic.LoadInt32(&vs.dead) != 0
}

// please don't change this function.
func (vs *ViewServer) GetRPCCount() int32 {
	return atomic.LoadInt32(&vs.rpccount)
}

func StartServer(me string) *ViewServer {
	vs := new(ViewServer)
	vs.me = me
	// Your vs.* initializations here.
    vs.view = View{0, "", ""}
    vs.primaryAck = 0
    vs.primaryTick = 0
    vs.backupAck = 0
    vs.backupTick = 0
    vs.currentTick = 0

	// tell net/rpc about our RPC server and handlers.
	rpcs := rpc.NewServer()
	rpcs.Register(vs)

	// prepare to receive connections from clients.
	// change "unix" to "tcp" to use over a network.
	os.Remove(vs.me) // only needed for "unix"
	l, e := net.Listen("unix", vs.me)
	if e != nil {
		log.Fatal("listen error: ", e)
	}
	vs.l = l

	// please don't change any of the following code,
	// or do anything to subvert it.

	// create a thread to accept RPC connections from clients.
	go func() {
		for vs.isdead() == false {
			conn, err := vs.l.Accept()
			if err == nil && vs.isdead() == false {
				atomic.AddInt32(&vs.rpccount, 1)
				go rpcs.ServeConn(conn)
			} else if err == nil {
				conn.Close()
			}
			if err != nil && vs.isdead() == false {
				fmt.Printf("ViewServer(%v) accept: %v\n", me, err.Error())
				vs.Kill()
			}
		}
	}()

	// create a thread to call tick() periodically.
	go func() {
		for vs.isdead() == false {
			vs.tick()
			time.Sleep(PingInterval)
		}
	}()

	return vs
}
