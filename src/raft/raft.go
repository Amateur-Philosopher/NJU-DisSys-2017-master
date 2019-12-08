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
import "math"
import "time"

import "bytes"
import "encoding/gob"
// import "fmt"

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
	LogEntry interface{}
	Term int
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
	isLeader    int // -1 for candidate, 0 for follower, 1 for leader
	
	heartbeatCh chan int // listen to heartbeat
	voteCh      chan int // listen to vote
	applyCh     chan ApplyMsg
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
func (rf *Raft) RequestVote(args RequestVoteArgs, reply *RequestVoteReply) {
	// Your code here.
	rf.mu.Lock()
	defer rf.persist()
	defer rf.mu.Unlock()

   
	// rf.voteCh <- 0

	reply.Term = rf.currentTerm
	reply.VoteGranted = false
	if args.Term < rf.currentTerm {//|| rf.isLeader == 1 {
		reply.VoteGranted = false
	}else {
		if args.Term > rf.currentTerm {
			rf.currentTerm = args.Term
			rf.VoteFor = -1
			rf.isLeader = 0
		}
		if rf.VoteFor == -1 || rf.VoteFor == args.CandidateId {
			lastLogIndex := len(rf.log) - 1
			lastLogTerm := 0
			if len(rf.log) > 0{
				lastLogTerm = rf.log[lastLogIndex].Term
			}
			if lastLogTerm < args.LastLogTerm {
				reply.VoteGranted = true
			} 
			if lastLogTerm == args.LastLogTerm && lastLogIndex <= args.LastLogIndex {
				reply.VoteGranted = true
			}

			if reply.VoteGranted{
				//rf.mu.Lock()
				rf.voteCh <- 0
				rf.VoteFor = args.CandidateId
				// rf.persist()
				//rf.mu.Unlock()
			}
			// fmt.Printf("id: %v, %v\n", rf.me, reply)
			return
		}
	}
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
func (rf *Raft) sendRequestVote(server int, args RequestVoteArgs, reply *RequestVoteReply) bool {
	// fmt.Printf("begin send %v %v\n", args, reply)
	ok := rf.peers[server].Call("Raft.RequestVote", args, reply)
	// fmt.Printf("send ok\n")
	//if reply.VoteGranted {
		//fmt.Printf("%d get vote from %d\n", rf.me, server)
	//}
	return ok
}

type AppendEntriesArgs struct {
	Term         int
	LeaderId     int
	PrevLogIndex int
	PrevLogTerm  int
	Entries      []LogEntries
	LeaderCommit int
}

type AppendEntriesReply struct {
	Term    int
	Success bool
}

// AppendEntries RPC handler
func (rf *Raft) AppendEntries(args AppendEntriesArgs, reply *AppendEntriesReply) {
	//fmt.Printf("%v\n", args)
	reply.Term = rf.currentTerm

	rf.mu.Lock()
	rf.heartbeatCh <- args.Term
	if rf.currentTerm < args.Term{
		rf.currentTerm = args.Term
		rf.isLeader = 0
		rf.VoteFor = -1
		//fmt.Printf("(((%v change to follower\n", rf.me)
	}
	//fmt.Printf("%d, %d\n", args.Term, args.LeaderId)
	if args.Term < rf.currentTerm {
		reply.Success = false
	}else {
		//fmt.Printf("%d\n", args.PrevLogIndex)
		if len(rf.log) - 1 < args.PrevLogIndex {
			reply.Success = false
		}else {
			reply.Success = true
			if args.PrevLogIndex > 0 && rf.log[args.PrevLogIndex].Term != args.PrevLogTerm{
				rf.log = rf.log[:args.PrevLogIndex]
				reply.Success = false
				// fmt.Printf("%d's log: %v\n", rf.me, rf.log)
			}
			//fmt.Printf("%v\n", args.Entries)
			for i:=1; i <= len(args.Entries); i++{
				if len(rf.log) - 1 < args.PrevLogIndex + i {
					rf.log = append(rf.log, args.Entries[i-1])
				}else{
					rf.log[args.PrevLogIndex+i] = args.Entries[i-1]
				}
			}
			if args.LeaderCommit > rf.commitIndex && len(args.Entries) > 0 && reply.Success {
				rf.commitIndex = int(math.Min(float64(args.LeaderCommit), float64(len(rf.log) - 1)))
				//fmt.Printf("%d %d %d\n", rf.lastApplied+1, rf.commitIndex, len(rf.log))
				for i:=rf.lastApplied+1; i <= rf.commitIndex; i++ {
					rf.applyCh <- ApplyMsg{i+1, rf.log[i].LogEntry, false, nil}
					rf.lastApplied = i
					// fmt.Printf("id:%d commit %d\n", rf.me, i)
				}
			}
		}
	}
	// reply.Term = rf.currentTerm
	// rf.VoteFor = -1
	rf.persist()
	rf.mu.Unlock()
}

func (rf *Raft) sendAppendEntries(server int, args AppendEntriesArgs, reply *AppendEntriesReply) bool {
	//fmt.Printf("%v\n", args)
	ok := rf.peers[server].Call("Raft.AppendEntries", args, reply)
	if ok {
		rf.mu.Lock()
		defer rf.persist()
		defer rf.mu.Unlock()
		if reply.Term > rf.currentTerm {
			rf.isLeader = 0
			return ok
		}
		if reply.Success {
			rf.nextIndex[server] = args.PrevLogIndex + len(args.Entries) + 1
			rf.matchIndex[server] = args.PrevLogIndex + len(args.Entries)
			// fmt.Printf("len(rf.log) %d, commitIndex %d\n", len(rf.log), rf.commitIndex)
			for i:=len(rf.log)-1; i > rf.commitIndex; i-- {
				countMatch := 0
				for j:=0; j < len(rf.peers); j++ {
					if rf.matchIndex[j] >= i {
						countMatch++
					}
				}
				// fmt.Printf("%d\n", countMatch)
				if countMatch * 2 > len(rf.peers) {
					rf.commitIndex = i
					// fmt.Printf("Leader %d commit index %d\n", rf.me, i)
					break
				}
			}
			for i:= rf.lastApplied + 1; i <= rf.commitIndex; i++ {
				rf.applyCh <- ApplyMsg{i+1, rf.log[i].LogEntry, false, nil}
				rf.lastApplied = i
			}
		}else {
			rf.nextIndex[server] -= 1
			//if rf.nextIndex[server] < 0 {
			//	rf.nextIndex[server] = 0
			//}
		}
	}
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
	index := rf.commitIndex
	term := rf.currentTerm
	isLeader := rf.isLeader == 1
	if isLeader {
		// fmt.Printf("%d got log %v \n", rf.me, command)
		newEntry := LogEntries{command, rf.currentTerm}
		rf.mu.Lock()
		rf.log = append(rf.log, newEntry)
		rf.nextIndex[rf.me] = len(rf.log)
		rf.matchIndex[rf.me] = len(rf.log) - 1
		index = len(rf.log)  // begin from 1 but we begin from 0 in programming
		// fmt.Printf("Leader %d log %v\n", rf.me, rf.log)
		rf.persist()
		rf.mu.Unlock()

		for i:=0; i < len(rf.peers); i++ {
			if i != rf.me && rf.isLeader == 1{
				go func(thisID int){
					lastLogIndex := len(rf.log) - 1
					argEntries := make([]LogEntries, 0)
					args := AppendEntriesArgs{rf.currentTerm, rf.me, -1, -1, argEntries, rf.commitIndex}
					if lastLogIndex >= rf.nextIndex[thisID] {
						prevLogIndex := rf.nextIndex[thisID] - 1
						prevLogTerm := -1
						if prevLogIndex >= 0 {
							prevLogTerm = rf.log[prevLogIndex].Term
						}
						args.PrevLogIndex = prevLogIndex
						args.PrevLogTerm = prevLogTerm
						// args.Entries = rf.log[rf.nextIndex[thisID]:]
						for ind:=rf.nextIndex[thisID]; ind < len(rf.log); ind++ {
							args.Entries = append(args.Entries, rf.log[ind])
						}
						args.LeaderCommit = rf.commitIndex
					}
					// fmt.Printf("%v\n", args)
					reply := &AppendEntriesReply{0, false}
					// fmt.Printf("%v send heartbeat to %v\n", rf.me, i)
					rf.sendAppendEntries(thisID, args, reply)
				}(i)
			}
		}
	}
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
	rf.mu.Lock()
	rf.nextIndex = make([]int, len(rf.peers))
	rf.matchIndex = make([]int, len(rf.peers))
	for i:=0; i < len(rf.peers); i++ {
		rf.nextIndex[i] = len(rf.log)
		rf.matchIndex[i] = -1
	}
	rf.persist()
	rf.mu.Unlock()
	
	chanReply := make(chan *AppendEntriesReply)
	for {
		for i:=0; i < len(rf.peers); i++ {
			if i != rf.me && rf.isLeader == 1{
				go func(thisID int){
					lastLogIndex := len(rf.log) - 1
					argEntries := make([]LogEntries, 0)
					args := AppendEntriesArgs{rf.currentTerm, rf.me, -1, -1, argEntries, rf.commitIndex}
					if lastLogIndex >= rf.nextIndex[thisID] || rf.matchIndex[thisID] < 0 {
					//if true {
						prevLogIndex := rf.nextIndex[thisID] - 1
						prevLogTerm := -1
						if prevLogIndex >= 0 {
							prevLogTerm = rf.log[prevLogIndex].Term
						}
						args.PrevLogIndex = prevLogIndex
						args.PrevLogTerm = prevLogTerm
						// args.Entries = rf.log[rf.nextIndex[thisID]:]

						for ind:=rf.nextIndex[thisID]; ind < len(rf.log); ind++ {
							args.Entries = append(args.Entries, rf.log[ind])
						}
						args.LeaderCommit = rf.commitIndex
					}
					// fmt.Printf("---%v\n", args)
					reply := &AppendEntriesReply{0, false}
					// fmt.Printf("%v send heartbeat to %v\n", rf.me, i)
					rf.sendAppendEntries(thisID, args, reply)
					chanReply <- reply
				}(i)
			}
		}
		time.Sleep(time.Millisecond * time.Duration(HEARTBEAT))
		if rf.isLeader != 1{
			return
		}
		//fmt.Printf("new heartbeat\n")
	}
}


func (rf *Raft) wantVote() {
	rf.mu.Lock()
	rf.isLeader = -1
	rf.currentTerm += 1
	rf.VoteFor = rf.me
	rf.persist()
	rf.mu.Unlock()

	lastLogIndex := len(rf.log) - 1
	lastLogTerm := 0
	if len(rf.log) > 0{
		lastLogTerm = rf.log[lastLogIndex].Term
	}
	getVoteNum := 1
	deadNodeNum := 0
	
	chanReply := make(chan *RequestVoteReply)
	chanOk := make(chan bool)
	for i:=0; i < len(rf.peers); i++ {
		if rf.isLeader != -1 {
			break
		}
		if rf.isLeader == -1 && i != rf.me {
			go func(thisID int){
				voteargs := RequestVoteArgs{Term:         rf.currentTerm, 
				                        CandidateId:  rf.me,
				                        LastLogIndex: lastLogIndex,
				                        LastLogTerm:  lastLogTerm}
				reply := &RequestVoteReply{Term:        -1, 
									   VoteGranted: false}
			
				// send vote request until get valid reply
				ok := rf.sendRequestVote(thisID, voteargs, reply)
				chanReply <- reply
				chanOk <- ok
			}(i)
		}
	}
	timer := time.NewTimer(getRandTimeout())
	for {
		select {
			case reply :=<- chanReply: {
				if reply.VoteGranted {
					getVoteNum += 1
					if rf.countVote(getVoteNum, deadNodeNum) {
						return
					}
				}else{
					if rf.currentTerm < reply.Term {
						rf.mu.Lock()
						rf.currentTerm = reply.Term
						rf.isLeader = 0
						// fmt.Printf("%d(term: %d) quit vote\n", rf.me, rf.currentTerm)
						rf.persist()
						rf.mu.Unlock()
						return
					}
				}
			}
			case ok :=<- chanOk: {
				if !ok{
					deadNodeNum += 1
					if rf.countVote(getVoteNum, deadNodeNum) {
						return
					}
				}
			}
			case hTerm := <- rf.heartbeatCh: {
				if hTerm >= rf.currentTerm{
					rf.mu.Lock()
					rf.isLeader = 0
					rf.persist()
					rf.mu.Unlock()
					return
				}
			}
			case <- timer.C: {
				go rf.wantVote()
				return
			}
		}
	}
}

func (rf *Raft) countVote(getVoteNum int, deadNodeNum int) bool {
	majorityNum := float32(len(rf.peers) - deadNodeNum) / 2.0
	// fmt.Printf("%d(term: %d) want become leader maj: %v vote: %v\n", rf.me, rf.currentTerm, majorityNum, getVoteNum)
	if float32(getVoteNum) > majorityNum && rf.isLeader == -1 && getVoteNum > 1{
		// You are the leader
		rf.mu.Lock()
		// fmt.Printf("%d become leader maj: %v vote: %v\n", rf.me, majorityNum, getVoteNum)
		rf.isLeader = 1
		rf.persist()
		rf.mu.Unlock()
		go rf.beLeader()
		return true
	}
	return false
}


// listen function for follower
func (rf *Raft) listen() {
	timer := time.NewTimer(getRandTimeout())
    for {
		select {
		case <- rf.heartbeatCh:
			timer.Reset(getRandTimeout())
		case <- rf.voteCh:
			// sb wants to be a leader
			//if rf.isLeader == 0{
			timer.Reset(getRandTimeout())
			//}
		case <- timer.C:
		    if rf.isLeader == 0 {
				// fmt.Printf("%d want to be leader\n", rf.me)
				go rf.wantVote()
			}	
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

	rf.commitIndex = -1
	rf.lastApplied = -1

	rf.isLeader = 0   // init as follower
	rf.heartbeatCh = make(chan int)
	rf.voteCh = make(chan int)
	rf.applyCh = applyCh
	// rf.applyCh <- ApplyMsg{}

	rf.readPersist(persister.ReadRaftState())

	go rf.listen()

	return rf
}
