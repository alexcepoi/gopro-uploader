package main

import (
	"log"
	"os/exec"
)

func checkDependencies(commands ...string) {
	for _, dep := range commands {
		if _, err := exec.LookPath(dep); err != nil {
			log.Fatalf("Could not find missing dependency %v :%v\n", dep, err)
		}
	}
}

func contains(list []string, elem string) bool {
	for _, i := range list {
		if i == elem {
			return true
		}
	}
	return false
}
