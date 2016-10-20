package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"runtime"
	"sync"
	"syscall"
	"time"
)

var restApiServer = flag.String("restApi", "", "listen addr for restapi")
var auth = flag.String("auth", "taven123", "restApi Password")
var gLocalConn net.Listener

var clientSMap map[string]net.Conn

var forwardInfo string

func main() {

	clientSMap = make(map[string]net.Conn)

	//解析传入的参数
	flag.Parse()

	if *restApiServer == "" {
		*restApiServer = "0.0.0.0:8000"
	}

	go StartHttpServer(*restApiServer)

	log.Println("restApiServer：", *restApiServer)
	fmt.Println("------------启动成功------------")

	//开启线程同步锁
	var w sync.WaitGroup
	w.Add(2)

	//开一个并发线程，接收退出信号
	go func() {
		c := make(chan os.Signal, 1)
		signal.Notify(c, os.Interrupt, syscall.SIGTERM)
		n := 0
		f := func() {
			<-c
			n++
			if n > 2 {
				log.Println("force shutdown")
				os.Exit(-1)
			}
			log.Println("received signal,shutdown")
			closeAllConn()
		}
		f()
		go func() {
			for {
				f()
			}
		}()
		//执行完成一次，Done() 等同于 Add(-1)，计数不为0，则阻塞
		w.Done()
	}()

	loop := func() {
		w.Done()

	}
	loop()
	w.Wait()

	fmt.Println("------------程序执行完成------------")

}

func StartHttpServer(addr string) {

	http.HandleFunc("/ServerSummary", ServerSummary)
	http.HandleFunc("/ForwardWork", ForwardWork)

	//
	err := http.ListenAndServe(addr, http.DefaultServeMux)

	if err != nil {
		fmt.Println("ListenAndServe error: ", err.Error())
	}

}

func ServerSummary(rw http.ResponseWriter, req *http.Request) {
	log.Println("ServerSummary")
	obj := make(map[string]interface{})
	obj["runtime_NumGoroutine"] = runtime.NumGoroutine()
	obj["runtime_GOOS"] = runtime.GOOS
	obj["runtime_GOARCH"] = runtime.GOARCH
	obj["restApi_Addr"] = *restApiServer
	obj["server_Time"] = time.Now()
	obj["clients_Count"] = len(clientSMap)

	var clist []string
	for cId, _ := range clientSMap {
		clist = append(clist, cId)
	}
	obj["clients_List"] = clist
	obj["forwardInfo"] = forwardInfo

	res, err := json.Marshal(obj)
	if err != nil {
		log.Println("json marshal:", err)
		return
	}

	rw.Header().Add("Content-Type", "application/json;charset=utf-8")
	_, err = rw.Write(res)
	if err != nil {
		log.Println("write err:", err)
	}
	return
}

func ForwardWork(rw http.ResponseWriter, req *http.Request) {
	req.ParseForm()

	obj := make(map[string]interface{})
	obj["code"] = 0
	obj["msg"] = ""

	paramAuth, hasAuth := req.Form["auth"]
	if !hasAuth {
		log.Println("request no auth")
		obj["code"] = 1
		obj["msg"] = "request no auth"
		responseResult(obj, rw)
		return

	}

	if paramAuth[0] != *auth {
		log.Println("request auth failed")
		obj["code"] = 1
		obj["msg"] = "request auth failed"
		responseResult(obj, rw)

		return
	}

	paramStatus, hasStatus := req.Form["status"]
	if !hasStatus {
		return

	}

	log.Println("param_status：", paramStatus)

	if paramStatus[0] == "1" {
		//启动服务
		paramFromAddr, hasFromAddr := req.Form["fromAddr"]
		paramToAddr, hasToAddr := req.Form["toAddr"]
		if gLocalConn != nil {
			gLocalConn.Close()
		}

		if hasFromAddr && hasToAddr {
			go forwardPort(paramFromAddr[0], paramToAddr[0])
		}
	}

	if paramStatus[0] == "0" {
		//关闭服务
		closeAllConn()
		forwardInfo = ""
	}

	responseResult(obj, rw)

	return

}

func responseResult(data map[string]interface{}, rw http.ResponseWriter) {
	res, err := json.Marshal(data)
	if err != nil {
		log.Println("json marshal:", err)
		return
	}

	rw.Header().Add("Content-Type", "application/json;charset=utf-8")
	_, err = rw.Write(res)
	if err != nil {
		log.Println("write err:", err)
	}
}

func closeAllConn() {
	for cId, conn := range clientSMap {
		log.Println("clientMap id：", cId)
		conn.Close()
		delete(clientSMap, cId)
	}

	if gLocalConn != nil {
		gLocalConn.Close()
		log.Println("Listener Close")
	} else {
		gLocalConn = nil
		log.Println("Listener set to nil", gLocalConn)
	}
}

func forwardPort(sourcePort string, targetPort string) {

	fmt.Println("sourcePort：", sourcePort, "targetPort：", targetPort)

	localConn, err := net.Listen("tcp", sourcePort)

	if err != nil {
		fmt.Println(err.Error())
		return
	}

	gLocalConn = localConn

	fmt.Println("服务启动成功，服务地址：", sourcePort)

	forwardInfo = fmt.Sprintf("%s - %s", sourcePort, targetPort)

	for {
		fmt.Println("Ready to Accept ...")
		sourceConn, err := gLocalConn.Accept()

		if err != nil {
			log.Println("server err:", err.Error())
			break
		}
		//log.Println("client", sc.id, "create session", sessionId)

		id := sourceConn.RemoteAddr().String()
		clientSMap[id] = sourceConn

		fmt.Println("conn.RemoteAddr().String() ：", id)

		//targetPort := "172.16.128.83:22"
		targetConn, err := net.DialTimeout("tcp", targetPort, 30*time.Second)

		go func() {
			_, err = io.Copy(targetConn, sourceConn)
			if err != nil {
				//log.Fatalf("io.Copy 1 failed: %v", err)
				fmt.Println("io.Copy 1 failed：", err.Error())
			}
		}()

		go func() {
			_, err = io.Copy(sourceConn, targetConn)
			if err != nil {
				//log.Fatalf("io.Copy 2 failed: %v", err)
				fmt.Println("io.Copy 2 failed：", err.Error())
			}
		}()

	}

	//
	log.Println("forwardPort end.")

}
