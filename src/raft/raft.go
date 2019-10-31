// 1. VoteFor有啥用 一个结点只能给一个人投票吗，如果是就没法保证有结点得到majority的投票了，如果不是则有很多结点有majority的投票了
// 1.1 应该是一个任期投一次，每个任期清理一下
// 1.2 什么时候更新任期？是请求投票的时候还是当选的时候？
// 2. 活着的结点数就是原来的节点数减去任期数（term）
package raft

//
// this is an outline of the API that raft must expose to
// the service (or tester). see comments below for
// each of these functions for more details.
//
// rf = Make(...)
//   create a new Raft server.
// rf.Start(command interface{}) (index, term, isleader)
//   start agreement on a new log entry
// rf.GetState() (term, isLeader)
//   ask a Raft for its current term, and whether it thinks it is leader
// ApplyMsg
//   each time a new entry is committed to the log, each Raft peer
//   should send an ApplyMsg to the service (or tester)
//   in the same server.
//

import "sync"
import "labrpc"
import "math/rand"
import "time"

import "bytes"
import "encoding/gob"
import "fmt"

const (
	HEARTBEAT = 100
	MIN_TIMEOUT = 150
	MAX_TIMEOUT = 300
)

//
// as each Raft peer becomes aware that successive log entries are
// committed, the peer should send an ApplyMsg to the service (or
// tester) on the same server, via the applyCh passed to Make().
//
type ApplyMsg struct {
	Index       int
	Command     interface{}
	UseSnapshot bool   // ignore for lab2; only used in lab3
	Snapshot    []byte // ignore for lab2; only used in lab3
}


type LogEntries struct {
	logEntries interface{}
	term int
}

//
// A Go object implementing a single Raft peer.
//
type Raft struct {
	mu        sync.Mutex
	peers     []*labrpc.ClientEnd
	persister *Persister
	me        int // index into peers[]

	// Your data here.
	// Look at the paper's Figure 2 for a description of what
	// state a Raft server must maintain.

	// Persistent state on all servers
	currentTerm int // lastest term server has seen(initializaed to 0)
	VoteFor     int // candidateId that received vote in current term, -1 for null
	log         []LogEntries // log entries(first index is 1)

	// Volatile state on all servers
	commitIndex int // index of highest log entry known to be committed(init 0)
	lastApplied int // index of highest log entry applied to state machine(init 0)

	// Volatile state on leaders (reinitialized after election)
	nextIndex  []int // for each server, index of the next log entry to send to that server(init to leader last log index + 1)
	matchIndex []int // for each server, index of highest log entry known to be replicated on server(init 0)
	
	// Vote state
	// getVoteNum     int // vote number the server get
	isLeader    int // -1 for candidate, 0 for follower, 1 for leader
	
	heartbeatCh chan int // listen to heartbeat
	voteCh      chan int // listen to vote

	// timer       *time.Timer
}

// return currentTerm and whether this server
// believes it is the leader.
func (rf *Raft) GetState() (int, bool) {

	var term int
	var isleader bool
	// Your code here.
	term = rf.currentTerm
	isleader = (rf.isLeader == 1)
	return term, isleader
}

//
// save Raft's persistent state to stable storage,
// where it can later be retrieved after a crash and restart.
// see paper's Figure 2 for a description of what should be persistent.
//
func (rf *Raft) persist() {
	// Your code here.
	// Example:
	// w := new(bytes.Buffer)
	// e := gob.NewEncoder(w)
	// e.Encode(rf.xxx)
	// e.Encode(rf.yyy)
	// data := w.Bytes()
	// rf.persister.SaveRaftState(data)
	w := new(bytes.Buffer)
	e := gob.NewEncoder(w)
	e.Encode(rf.currentTerm)
	e.Encode(rf.VoteFor)
	e.Encode(rf.log)
	data := w.Bytes()
	rf.persister.SaveRaftState(data)
}

//
// restore previously persisted state.
//
func (rf *Raft) readPersist(data []byte) {
	// Your code here.
	// Example:
	// r := bytes.NewBuffer(data)
	// d := gob.NewDecoder(r)
	// d.Decode(&rf.xxx)
	// d.Decode(&rf.yyy)
	r := bytes.NewBuffer(data)
	d := gob.NewDecoder(r)
	d.Decode(&rf.currentTerm)
	d.Decode(&rf.VoteFor)
	d.Decode(&rf.log)
}




//
// example RequestVote RPC arguments structure.
//
type RequestVoteArgs struct {
	// Your data here.
	Term         int // candidate's term
	CandidateId  int //candidate requesting vote
	LastLogIndex int // index of candidate's last log entry
	LastLogTerm  int // term of candidate's last log entry
}

//
// example RequestVote RPC reply structure.
//
type RequestVoteReply struct {
	// Your data here.
	Term        int // current Term, for candidate to update itself
	VoteGranted bool // true means candidate received vote
}

//
// example RequestVote RPC handler.
//
func (rf *Raft) RequestVote(args *RequestVoteArgs, reply *RequestVoteReply) {
	// Your code here.
   
	// fmt.Printf("request Vote %v %v\n", args, reply)
	rf.voteCh <- 0

	reply.Term = rf.currentTerm
	if args.Term < rf.currentTerm{
		reply.VoteGranted = false
	}else{
		if args.Term > rf.currentTerm && rf.isLeader != 1{
			// new term, reset VoteFor
			rf.mu.Lock()
			rf.VoteFor = -1
			rf.currentTerm = args.Term
			rf.persist()
			rf.isLeader = 0
			fmt.Printf("(((%v change to follower\n", rf.me)
			rf.mu.Unlock()
		}
		if rf.VoteFor == -1 || rf.VoteFor == args.CandidateId{
			if args.LastLogIndex >= rf.commitIndex{
				reply.VoteGranted = true
				rf.mu.Lock()
				rf.VoteFor = args.CandidateId
				rf.persist()
				// rf.currentTerm = args.Term
				rf.mu.Unlock()
				fmt.Printf("id: %v, %v\n", rf.me, reply)
				return
			}
		}
	}
	reply.VoteGranted = false
}

//
// example code to send a RequestVote RPC to a server.
// server is the index of the target server in rf.peers[].
// expects RPC arguments in args.
// fills in *reply with RPC reply, so caller should
// pass &reply.
// the types of the args and reply passed to Call() must be
// the same as the types of the arguments declared in the
// handler function (including whether they are pointers).
//
// returns true if labrpc says the RPC was delivered.
//
// if you're having trouble getting RPC to work, check that you've
// capitalized all field names in structs passed over RPC, and
// that the caller passes the address of the reply struct with &, not
// the struct itself.
//
func (rf *Raft) sendRequestVote(server int, args *RequestVoteArgs, reply *RequestVoteReply) bool {
	// fmt.Printf("begin send %v %v\n", args, reply)
	ok := rf.peers[server].Call("Raft.RequestVote", args, reply)
	// fmt.Printf("send ok\n")
	return ok
}

type AppendEntriesArgs struct {
	Term     int
	leaderId int
}

type AppendEntriesReply struct {
	Term int
}

// AppedfEntries RPC handler
func (rf *Raft) AppendEntries(args *AppendEntriesArgs, reply *AppendEntriesReply) {
	rf.heartbeatCh <- args.leaderId
	rf.currentTerm = args.Term
	rf.VoteFor = -1
	rf.persist()
}

func (rf *Raft) sendAppendEntries(server int, args *AppendEntriesArgs, reply *AppendEntriesReply) bool {
	ok := rf.peers[server].Call("Raft.AppendEntries", args, reply)
	return ok
}

//
// the service using Raft (e.g. a k/v server) wants to start
// agreement on the next command to be appended to Raft's log. if this
// server isn't the leader, returns false. otherwise start the
// agreement and return immediately. there is no guarantee that this
// command will ever be committed to the Raft log, since the leader
// may fail or lose an election.
//
// the first return value is the index that the command will appear at
// if it's ever committed. the second return value is the current
// term. the third return value is true if this server believes it is
// the leader.
//
func (rf *Raft) Start(command interface{}) (int, int, bool) {
	index := -1
	term := -1
	isLeader := true


	return index, term, isLeader
}

//
// the tester calls Kill() when a Raft instance won't
// be needed again. you are not required to do anything
// in Kill(), but it might be convenient to (for example)
// turn off debug output from this instance.
//
func (rf *Raft) Kill() {
	// Your code here, if desired.
}


func getRandTimeout() time.Duration{
	randTimeOut := MIN_TIMEOUT + int(rand.Float32() * (MAX_TIMEOUT - MIN_TIMEOUT))
	return time.Millisecond * time.Duration(randTimeOut)
}


func (rf *Raft) beLeader() {
	fmt.Printf("%v become leader\n", rf.me)
	rf.mu.Lock()
	rf.isLeader = 1
	rf.mu.Unlock()
	for {
		// args := AppendEntriesArgs{term: rf.currentTerm}
		for i:=0; i < len(rf.peers); i++ {
			if rf.isLeader != 1 {
				fmt.Printf("%v no longer be leader", rf.me)
				return
			}

			if i != rf.me {
				args := AppendEntriesArgs{Term: rf.currentTerm, leaderId: rf.me}
				reply := AppendEntriesReply{0}
				// fmt.Printf("%v send heartbeat to %v\n", rf.me, i)
				rf.sendAppendEntries(i, &args, &reply)
			}
		}
		time.Sleep(time.Millisecond * time.Duration(HEARTBEAT))
	}
}


func (rf *Raft) wantVote() {
	// fmt.Printf("%v wants to be leader\n", rf.me)
	rf.mu.Lock()
	rf.isLeader = -1
	rf.currentTerm += 1
	rf.VoteFor = rf.me
	rf.persist()
	// rf.getVoteNum = 1  // getVoteNum as a local variable may be enough
	rf.mu.Unlock()
	
	lastLogTerm := 0
	if len(rf.log) > 0{
		lastLogTerm = rf.log[rf.commitIndex].term
	}
	getVoteNum := 1
	deadNodeNum := 0
	// fmt.Printf("%v peers\n", len(rf.peers))
	for i:=0; i < len(rf.peers); i++ {
		if rf.isLeader != -1 {
			break
		}
		if rf.isLeader == -1 && i != rf.me {
			voteargs := RequestVoteArgs{Term:         rf.currentTerm, 
				                        CandidateId:  rf.me,
				                        LastLogIndex: rf.commitIndex,
				                        LastLogTerm:  lastLogTerm}
			reply := RequestVoteReply{Term:        -1, 
									   VoteGranted: false}
			
			// send vote request until get valid reply
			// fmt.Printf("send request vote to %v %v % v\n", i, reply, voteargs)
			get_reply := rf.sendRequestVote(i, &voteargs, &reply)
			// fmt.Printf("%v\n", get_reply)
			
			if !get_reply {
				deadNodeNum += 1
			}else{
				if reply.VoteGranted {
					getVoteNum += 1
				}else{
					// this server's term may not the lastest term
					// Not certain!!!
					if rf.currentTerm < reply.Term {
						rf.currentTerm = reply.Term
						rf.isLeader = 0
						fmt.Printf("(((%v change to follower\n", rf.me)
						return
					}

				}
			}
		}
	}

	majorityNum := float32(len(rf.peers) - deadNodeNum) / 2.0 // alive node number
	// fmt.Printf("major: %v, getV %v, term %v\n", majorityNum, getVoteNum, rf.currentTerm)
	if rf.isLeader != -1 {
		// print("quit\n")
		return 
	}
	if float32(getVoteNum) > majorityNum && rf.isLeader == -1 {
		// You are the leader
		go rf.beLeader()
	}else{
		if rf.isLeader == -1{
			rf.wantVote()
		}
	}
}


// listen function for follower
func (rf *Raft) listen() {
	timer := time.NewTimer(getRandTimeout())
    for {
		// laId := 0
		select {
		case <- rf.heartbeatCh:
			// nothing happen
			// fmt.Printf("feel heartbeat from %v\n", laId)
			if rf.me == 2 {
				fmt.Printf("2 feel heartbeat %v\n", rf.isLeader)
			}
			if rf.isLeader != 0{
				rf.mu.Lock()
			    rf.isLeader = 0
			    fmt.Printf("(((%v change to follower\n", rf.me)
			    rf.mu.Unlock()
			}
			timer.Reset(getRandTimeout())
		case <- rf.voteCh:
			// sb wants to be a leader
			if rf.isLeader == 0{
				timer.Reset(getRandTimeout())
			}
			// timer.Reset(getRandTimeout())
		case <- timer.C:
		    if rf.isLeader == 0 {
				// fmt.Printf("begin from listen\n")
				go rf.wantVote()
			}	
			// go rf.wantVote()
		}
	}
}

//
// the service or tester wants to create a Raft server. the ports
// of all the Raft servers (including this one) are in peers[]. this
// server's port is peers[me]. all the servers' peers[] arrays
// have the same order. persister is a place for this server to
// save its persistent state, and also initially holds the most
// recent saved state, if any. applyCh is a channel on which the
// tester or service expects Raft to send ApplyMsg messages.
// Make() must return quickly, so it should start goroutines
// for any long-running work.
//
func Make(peers []*labrpc.ClientEnd, me int,
	persister *Persister, applyCh chan ApplyMsg) *Raft {
	rf := &Raft{}
	rf.peers = peers
	rf.persister = persister
	rf.me = me

	// Your initialization code here.
	rf.currentTerm = 0
	rf.VoteFor = -1
	rf.log = make([]LogEntries, 0)

	rf.commitIndex = 0
	rf.lastApplied = 0

	// rf.getVoteNum = 0
	rf.isLeader = 0   // init as follower
	rf.heartbeatCh = make(chan int)
	rf.voteCh = make(chan int)

	// timeout := time.Millisecond * getRandTimeout()
	// rf.timer = time.NewTimer(timeout)
	// initialize from state persisted before a crash
	rf.readPersist(persister.ReadRaftState())

	go rf.listen()

	return rf
}
