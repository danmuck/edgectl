package main

// serve.go is the dev-server front end for the lab stack (docs/progress/lab_mode.md): the same
// bootstrap as lab mode, but Mirage's output streams to this terminal (and to the lab log) and
// there is no menu. The
// stack is driven from outside over api/v1 (docs/architecture/definitions/api_v1.toml).

import (
	"errors"
	"os"
	"os/signal"
	"syscall"

	logs "github.com/danmuck/smplog"
)

// Entry point for -mode serve: bring the lab up, stream Mirage's output until Ctrl-C or until
// Mirage exits, and always stop Mirage on the way out. Its own lines go through smplog like the
// runtime's, so level and color follow one environment (EDGECTL_LOG_LEVEL, EDGECTL_LOG_NOCOLOR).
func runServeMode(dir string) error {
	logs.Infof("serve starting (loopback only) dir=%q", dir)
	lab, err := ensureLab(dir)
	if err != nil {
		return err
	}
	for _, note := range lab.notes {
		logs.Infof("serve %s", note)
	}
	logs.Infof("serve building miragectl from this checkout")
	if err := lab.build(); err != nil {
		return err
	}
	logs.Infof("serve api/v1         https://%s/api/v1 (mutual TLS)", lab.apiV1Addr)
	logs.Infof("serve admin          %s (mutual TLS)", lab.mirageAdminAddr)
	logs.Infof("serve session        %s", lab.sessionAddr)
	logs.Infof("serve ghost          %s admin %s", labGhostID, lab.ghostAdminAddr)
	logs.Infof("serve operator cert  %s", lab.display("certs", labOperator+".crt"))
	logs.Infof("serve operator key   %s", lab.display("certs", labOperator+".key"))
	logs.Infof("serve CA             %s", lab.display("certs", "ca.crt"))
	logs.Infof("serve log            %s", lab.display("mirage.log"))
	logs.Warnf("serve running: Ctrl-C to stop")

	// Registered before launch so a signal during startup still stops Mirage.
	sig := make(chan os.Signal, 1)
	signal.Notify(sig, os.Interrupt, syscall.SIGTERM, syscall.SIGHUP)
	if err := lab.startTee(); err != nil {
		return err
	}
	defer lab.stop()
	select {
	case s := <-sig:
		logs.Warnf("serve stopping mirage signal=%s", s)
		return nil
	case <-lab.exited:
		return errors.New("mirage exited on its own; see the output above")
	}
}
