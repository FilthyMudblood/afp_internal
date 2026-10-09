package main

import (
	"context"
	"errors"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"github.com/FilthyMudblood/aegis-fabric/internal/config"
	"github.com/FilthyMudblood/aegis-fabric/internal/core"
	"github.com/FilthyMudblood/aegis-fabric/internal/dataplane"
	"github.com/FilthyMudblood/aegis-fabric/internal/ipc"
	"github.com/FilthyMudblood/aegis-fabric/internal/policyplane"
	"github.com/FilthyMudblood/aegis-fabric/internal/topology"
	afpv1 "github.com/FilthyMudblood/aegis-fabric/pkg/protocol/v1" // 假定 protobuf 生成的包位置
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"google.golang.org/protobuf/proto"
)

func main() {
	// 1. Initialize structured logger for infrastructure telemetry
	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelDebug}))
	slog.SetDefault(logger)

	// 2. Setup graceful shutdown context
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	runtimeCfg := config.LoadEnvConfig()

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		sig := <-sigCh
		slog.Info("received termination signal, initiating graceful shutdown", "signal", sig.String())
		cancel()
	}()

	// 3. Initialize Topology Storage and Bootstrap Network
	store := topology.NewInMemoryNeighborStore()

	cfgPath := os.Getenv("AFP_BOOTSTRAP_PATH")
	if cfgPath == "" {
		cfgPath = "bootstrap.json"
	}

	bootstrapCfg, err := config.LoadBootstrapConfig(cfgPath)
	if err != nil {
		// Support running from project root via `go run ./cmd/sidecar`.
		if cfgPath == "bootstrap.json" {
			cfgPath = "cmd/sidecar/bootstrap.json"
			bootstrapCfg, err = config.LoadBootstrapConfig(cfgPath)
		}
		if err != nil {
			// Infrastructure constraint: Cannot start an isolated proxy without genesis topology.
			slog.Error("failed to load bootstrap config, halting startup", "error", err, "path", cfgPath)
			os.Exit(1)
		}
	}

	for _, seed := range bootstrapCfg.SeedNodes {
		store.UpsertNeighbor(seed.PeerID, seed.CVP)
		if seed.Endpoint != "" {
			store.UpsertEndpoint(seed.PeerID, seed.Endpoint)
		}
	}
	slog.Info("topology bootstrapped successfully", "core_neighbors_loaded", len(bootstrapCfg.SeedNodes), "run_mode", runtimeCfg.Mode)

	// 4. Initialize Gossip and Single Execution Authority
	localDID := os.Getenv("AFP_LOCAL_DID")
	if localDID == "" {
		localDID = "did:afp:sidecar:local"
	}
	identity, err := topology.GenerateIdentity(localDID)
	if err != nil {
		slog.Error("failed to generate gossip identity", "error", err)
		os.Exit(1)
	}
	gossip := topology.NewGossipBroadcaster(identity, store, topology.NewPublicKeyDirectory())
	slog.Info("gossip identity ready", "local_did", identity.DID)
	resolver := topology.NewResolver(store)

	epochClock := func() uint64 {
		return uint64(time.Now().Unix())
	}
	entropyMonitor := core.NewEntropyMonitor(&runtimeCfg.Core, &core.CgroupsMetricsProvider{})
	sea := dataplane.NewSingleExecutionAuthority(runtimeCfg, entropyMonitor, epochClock, gossip)

	policyDir := os.Getenv("AFP_POLICY_DIR")
	if policyDir == "" {
		policyDir = "/etc/afp/policy"
	}
	if err := sea.RuntimePolicy().WatchPolicyDir(policyDir); err != nil {
		slog.Warn("policy hot-reload watcher disabled", "dir", policyDir, "error", err)
	} else {
		slog.Info("policy hot-reload watcher enabled", "dir", policyDir)
	}

	policyControllerAddr := os.Getenv("AFP_POLICY_CONTROLLER_ADDR")
	if policyControllerAddr != "" {
		policyNamespace := os.Getenv("AFP_NAMESPACE")
		if policyNamespace == "" {
			policyNamespace = "afp-system"
		}
		streamClient := policyplane.NewStreamClient(policyControllerAddr, localDID, policyNamespace, sea.RuntimePolicy())
		go func() {
			if err := streamClient.Run(ctx); err != nil && ctx.Err() == nil {
				slog.Error("policy stream client stopped", "error", err, "controller", policyControllerAddr)
			}
		}()
		slog.Info("policy stream client enabled", "controller", policyControllerAddr)
	}

	ipcSocket := os.Getenv("AFP_IPC_SOCKET")
	if ipcSocket == "" {
		ipcSocket = "/var/run/afp/agent.sock"
	}
	ipcServer := ipc.NewServer(sea, ipcSocket)
	go func() {
		if err := ipcServer.Serve(ctx); err != nil {
			slog.Error("sdk ipc server stopped", "error", err, "socket", ipcSocket)
		}
	}()

	var wg sync.WaitGroup

	metricsAddr := os.Getenv("AFP_METRICS_ADDR")
	if metricsAddr == "" {
		metricsAddr = "127.0.0.1:9090"
	}
	metricsMux := http.NewServeMux()
	metricsMux.Handle("/metrics", promhttp.Handler())
	metricsServer := &http.Server{
		Addr:    metricsAddr,
		Handler: metricsMux,
	}
	go func() {
		if err := metricsServer.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			slog.Error("metrics server failed", "error", err)
		}
	}()
	slog.Info("AFP telemetry endpoint started", "address", metricsAddr, "path", "/metrics")

	// --- Egress Router Setup ---
	// 模拟一个轻量级的本地资源熵压采样器 (例如读取 CPU 负载，此处直接返回安全值 0.3)
	entropySampler := func() float32 { return entropyMonitor.CalculateCoreEntropy() }
	egressRouter := dataplane.NewEgressRouter(localDID, epochClock, entropySampler)

	// Sidecar 必须开启一个仅绑定在 Loopback (127.0.0.1) 的内部端口，专供本地 AgentOS 发送对外请求
	egressListenAddr := os.Getenv("AFP_EGRESS_ADDR")
	if egressListenAddr == "" {
		egressListenAddr = "127.0.0.1:8081"
	}
	egressListener, err := net.Listen("tcp", egressListenAddr)
	if err != nil {
		slog.Error("failed to bind egress listener", "error", err)
		os.Exit(1)
	}
	defer egressListener.Close()
	slog.Info("AFP Sidecar Egress started (Local Agent Interface)", "address", egressListenAddr)

	go func() {
		for {
			conn, err := egressListener.Accept()
			if err != nil {
				if errors.Is(err, net.ErrClosed) {
					break
				}
				continue
			}

			wg.Add(1)
			go func(c net.Conn) {
				defer wg.Done()
				defer c.Close()

				// 1. Read trace context + target DID + payload from local AgentOS
				traceID, currentDepth, targetDID, payload, err := dataplane.ReadLocalEgressRequest(c)
				if err != nil {
					slog.Error("failed to read local outbound payload", "error", err)
					return
				}

				// 2. Resolve target DID into remote sidecar endpoint from recursive topology discovery
				targetMeshIP, err := resolver.Resolve(ctx, targetDID)
				if err != nil {
					slog.Error("egress dispatch failed: target resolution exhausted", "target", targetDID, "error", err)
					return
				}

				if err := egressRouter.Dispatch(ctx, targetMeshIP, traceID, currentDepth, payload); err != nil {
					slog.Warn("egress dispatch failed", "target", targetMeshIP, "target_did", targetDID, "error", err)
				}
			}(conn)
		}
	}()
	// ---------------------------

	// 5. Start raw TCP ingress listener for the Sidecar
	listenAddr := os.Getenv("AFP_INGRESS_ADDR")
	if listenAddr == "" {
		listenAddr = ":8080"
	}
	listener, err := net.Listen("tcp", listenAddr)
	if err != nil {
		slog.Error("failed to bind ingress listener", "error", err)
		os.Exit(1)
	}
	defer listener.Close()

	slog.Info("AFP Sidecar Ingress started", "address", listenAddr)

	// 6. Main Accept Loop
	go func() {
		for {
			conn, err := listener.Accept()
			if err != nil {
				if errors.Is(err, net.ErrClosed) {
					break // Normal shutdown trigger
				}
				slog.Error("socket accept error", "error", err)
				continue
			}

			wg.Add(1)
			go func(c net.Conn) {
				defer wg.Done()
				defer c.Close()

				peerIP := c.RemoteAddr().(*net.TCPAddr).IP.String()

				// 1. Defend against TCP stream fragmentation
				payload, err := dataplane.ReadFrame(c)
				if err != nil {
					slog.Warn("socket read/frame error", "peer", peerIP, "error", err)
					return
				}

				// 2. Zero-trust Protobuf Unmarshaling
				var header afpv1.GovernanceHeader
				if err := proto.Unmarshal(payload, &header); err != nil {
					slog.Warn("malformed governance header", "peer", peerIP, "error", err)
					return
				}

				// 3. Enforce Governance via Single Execution Authority
				err = sea.HandleIngress(ctx, peerIP, &header)
				if err != nil {
					// Error Classification and Telemetry
					switch {
					case errors.Is(err, dataplane.ErrTopologyIsolated):
						slog.Warn("connection physically dropped: topology isolated", "peer", peerIP)
					case errors.Is(err, dataplane.ErrProbationReject):
						slog.Debug("connection physically dropped: probation frequency limited", "peer", peerIP)
					case errors.Is(err, dataplane.ErrStrangerTaxFailed):
						slog.Warn("connection dropped: stranger tax failed", "peer", peerIP)
					default:
						slog.Error("connection physically dropped: unhandled authority error", "peer", peerIP, "error", err)
					}
					return // Immediate socket closure, severing physical path
				}

				// 7. Fast Path or Delayed Slow Path passed.
				slog.Debug("governance header validated, traffic permitted to local agent", "peer", peerIP)
				// TODO: io.Copy to local AgentOS runtime / Noesis core.
			}(conn)
		}
	}()

	// 7. Wait for Context Cancellation and Drain
	<-ctx.Done()
	slog.Info("shutting down listener...")
	ipcServer.Stop()
	_ = metricsServer.Shutdown(context.Background())
	egressListener.Close()
	listener.Close() // Breaks the Accept() loop

	waitCh := make(chan struct{})
	go func() {
		wg.Wait()
		close(waitCh)
	}()

	select {
	case <-waitCh:
		slog.Info("all active proxy connections drained gracefully")
	case <-time.After(5 * time.Second):
		slog.Warn("timeout waiting for connections to drain, forcing process exit")
	}
}
