package main

import (
	"strings"
	"log"
)

func main() {


	contain := strings.Contains("{\"type\":\"message\",\"channel\":\"C0NAH12SJ\",\"user\":\"U0F3WCD29\",\"text\":\"reviews show open\",\"ts\":\"1456333255.000005\",\"team\":\"T02QGTRD1\"}", "reviews show open")
	log.Println("contain", contain)

}
