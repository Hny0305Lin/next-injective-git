// Command derive-address-demo prints the bech32 form of an EVM address so
// operators can compose an igit:// remote URL for an EVM-owned repository.
// It is read-only and never touches a key or the network.
package main

import (
	"flag"
	"fmt"
	"os"
	"strings"

	"github.com/Hny0305Lin/next-injective-git/cli/internal/chain"
)

func main() {
	var address string
	flag.StringVar(&address, "address", "", "EVM (0x...) or bech32 (inj1...) address")
	flag.Parse()
	if strings.TrimSpace(address) == "" {
		fmt.Fprintln(os.Stderr, "-address is required")
		os.Exit(1)
	}
	evm, err := chain.NormalizeEVMAddress(address)
	if err != nil {
		fmt.Fprintf(os.Stderr, "normalize: %v\n", err)
		os.Exit(1)
	}
	bech, err := chain.UserAddressFromEVM(address)
	if err != nil {
		fmt.Fprintf(os.Stderr, "bech32: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("evm=%s\nbech32=%s\n", evm, bech)
}
