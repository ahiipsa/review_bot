package main
import (
	"log"
//	"time"
	"fmt"
	"golang.org/x/net/websocket"
	"slack"
)



func firstDigist(num int) int{
	if num / 10 == 0 {
		return num
	}

	return firstDigist(num/10)
}

func lastDigist(num int) int {
	return num%10
}

func main() {

//	tiker := time.NewTicker(60 * time.Second)
//	for _ = range tiker.C {
//		log.Println("step")
//	}






}
