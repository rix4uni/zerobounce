package banner

import (
	"fmt"
)

// prints the version message
const version = "v0.0.1"

func PrintVersion() {
	fmt.Printf("Current zerobounce version %s\n", version)
}

// Prints the Colorful banner
func PrintBanner() {
	banner := `			   __                                   
 ____  ___   _____ ____   / /_   ____   __  __ ____   _____ ___ 
/_  / / _ \ / ___// __ \ / __ \ / __ \ / / / // __ \ / ___// _ \
 / /_/  __// /   / /_/ // /_/ // /_/ // /_/ // / / // /__ /  __/
/___/\___//_/    \____//_.___/ \____/ \__,_//_/ /_/ \___/ \___/
`
	fmt.Printf("%s\n%55s\n\n", banner, "Current zerobounce version "+version)
}
