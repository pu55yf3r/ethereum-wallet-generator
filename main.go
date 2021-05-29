package main

import (
	"flag"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/cheggaaa/pb/v3"
	hdwallet "github.com/miguelmota/go-ethereum-hdwallet"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

var wg sync.WaitGroup

type Wallet struct {
	Address    string
	PrivateKey string
	Mnemonic   string
	Bits       int
	HDPath     string
	CreatedAt  time.Time
	gorm.Model
}

func NewWallet(bits int, hdPath string) *Wallet {
	mnemonic, _ := hdwallet.NewMnemonic(bits)

	return &Wallet{
		Mnemonic:  mnemonic,
		HDPath:    hdPath,
		CreatedAt: time.Now(),
	}
}

func (w *Wallet) createWallet(mnemonic string) *Wallet {
	wallet, _ := hdwallet.NewFromMnemonic(w.Mnemonic)

	path := hdwallet.DefaultBaseDerivationPath
	if w.HDPath != "" {
		path = hdwallet.MustParseDerivationPath(w.HDPath)
	}

	account, _ := wallet.Derive(path, false)
	pk, _ := wallet.PrivateKeyHex(account)

	w.Address = account.Address.Hex()
	w.PrivateKey = pk
	w.UpdatedAt = time.Now()

	return w
}

func generateNewWallet(bits int) *Wallet {
	mnemonic, _ := hdwallet.NewMnemonic(bits)
	wallet := createWallet(mnemonic)
	wallet.Bits = bits
	return wallet
}

func createWallet(mnemonic string) *Wallet {
	wallet, _ := hdwallet.NewFromMnemonic(mnemonic)

	account, _ := wallet.Derive(hdwallet.DefaultBaseDerivationPath, false)
	pk, _ := wallet.PrivateKeyHex(account)

	return &Wallet{
		Address:    account.Address.Hex(),
		PrivateKey: pk,
		Mnemonic:   mnemonic,
		HDPath:     account.URL.Path,
		CreatedAt:  time.Now(),
	}
}

func main() {

	interrupt := make(chan os.Signal, 1)

	signal.Notify(
		interrupt,
		syscall.SIGHUP,  // kill -SIGHUP XXXX
		syscall.SIGINT,  // kill -SIGINT XXXX or Ctrl+c
		syscall.SIGQUIT, // kill -SIGQUIT XXXX
		syscall.SIGTERM, // kill -SIGTERM XXXX
	)

	fmt.Println("===============ETH Wallet Generator===============")
	fmt.Println(" ")

	number := flag.Int("n", 10, "set number of wallets to generate")
	dbPath := flag.String("db", "", "set sqlite output path eg. wallets.db")
	concurrency := flag.Int("c", 1, "set number of concurrency")
	bits := flag.Int("bit", 256, "set number of entropy bits [128, 256]")
	contain := flag.String("contain", "", "used to check the given letters present in the given string or not")
	isDryrun := flag.Bool("dryrun", false, "generate wallet without result (used for benchamark speed)")
	flag.Parse()

	now := time.Now()
	resolvedCount := 0

	defer func() {
		fmt.Printf("\nResolved Speed: %.2f w/s\n", float64(resolvedCount)/time.Since(now).Seconds())
		fmt.Printf("Total Duration: %v\n", time.Since(now))
		fmt.Printf("Total Wallet Resolved: %d w\n", resolvedCount)

		fmt.Printf("\nCopyright (C) 2021 Planxnx <planxthanee@gmail.com>\n")
	}()

	go func() {
		bar := pb.StartNew(*number)
		bar.SetTemplate(pb.Default)
		bar.SetTemplateString(`{{counters . }} | {{bar . "" "█" "█" "" "" | rndcolor}} | {{percent . }} | {{speed . }} | {{string . "resolved"}}`)
		defer func() {
			interrupt <- syscall.SIGQUIT
		}()

		if *dbPath != "" {
			db, err := gorm.Open(sqlite.Open(*dbPath), &gorm.Config{
				Logger: logger.Default.LogMode(logger.Silent),
				DryRun: *isDryrun,
			})
			if err != nil {
				panic(err)
			}

			if !*isDryrun {
				db.AutoMigrate(&Wallet{})
			}

			for i := 0; i < *number; i += *concurrency {
				tx := db.Begin()
				txResolved := 0
				for j := 0; j < *concurrency && i+j < *number; j++ {
					wg.Add(1)

					go func(j int) {
						defer wg.Done()

						wallet := generateNewWallet(*bits)
						bar.Increment()

						if *contain != "" && !strings.Contains(wallet.Address, *contain) {
							return
						}

						txResolved++
						tx.Create(wallet)
					}(j)
				}
				wg.Wait()
				tx.Commit()
				resolvedCount += txResolved
				bar.Set("resolved", fmt.Sprintf("resovled: %v", resolvedCount))
			}
			bar.Finish()
			return
		}

		var result strings.Builder

		for i := 0; i < *number; i += *concurrency {
			for j := 0; j < *concurrency && i+j < *number; j++ {
				wg.Add(1)

				go func(j int) {
					defer wg.Done()

					wallet := generateNewWallet(*bits)
					bar.Increment()

					if *contain != "" && !strings.Contains(wallet.Address, *contain) {
						return
					}

					fmt.Fprintf(&result, "%-18s %s\n", wallet.Address, wallet.Mnemonic)
					resolvedCount++
					bar.Set("resolved", fmt.Sprintf("resovled: %v", resolvedCount))
				}(j)
			}
			wg.Wait()
		}
		bar.Finish()

		if *isDryrun {
			return
		}

		fmt.Printf("\n%-42s %s\n", "Address", "Seed")
		fmt.Printf("%-42s %s\n", strings.Repeat("-", 42), strings.Repeat("-", 160))
		fmt.Println(result.String())
	}()
	<-interrupt
}
