package ml

import (
	"errors"
	"strings"

	"google.golang.org/grpc"
	_ "google.golang.org/grpc/balancer/roundrobin"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/resolver"
	"google.golang.org/grpc/resolver/manual"
)

// NewConnection uses round-robin across an explicit comma-separated list of
// local ML replicas. A single address retains normal gRPC resolver behavior.
func NewConnection(address string) (*grpc.ClientConn, error) {
	opts := []grpc.DialOption{grpc.WithTransportCredentials(insecure.NewCredentials())}
	if !strings.Contains(address, ",") {
		return grpc.NewClient(address, opts...)
	}
	var addresses []resolver.Address
	for _, value := range strings.Split(address, ",") {
		value = strings.TrimSpace(value)
		if value == "" {
			return nil, errors.New("empty ML replica address")
		}
		addresses = append(addresses, resolver.Address{Addr: value})
	}
	r := manual.NewBuilderWithScheme("mlreplicas")
	r.InitialState(resolver.State{Addresses: addresses})
	opts = append(opts, grpc.WithResolvers(r), grpc.WithDefaultServiceConfig(`{"loadBalancingConfig":[{"round_robin":{}}]}`))
	return grpc.NewClient(r.Scheme()+":///local", opts...)
}
