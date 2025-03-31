// Package ibm implements methods described in the [cloud] package for interacting with IBM cloud instances.
// Currently System Z and Power Systems instances are supported with a Virtual Private Cloud (VPC) running System
// Z virtual server instances and a Power Virtual Server Workspace running Power Systems virtual server instances.
//
// All methods of the CloudProvider interface are implemented.
package ibm

import (
	"context"
	"encoding/base64"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/go-logr/logr"
	"github.com/konflux-ci/multi-platform-controller/pkg/cloud"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const maxPPCNameLength = 47

// CreateIBMPowerCloudConfig returns an IBM Power Systems cloud configuration that implements the CloudProvider interface.
func CreateIBMPowerCloudConfig(platform string, config map[string]string, systemNamespace string) cloud.CloudProvider {
	mem, err := strconv.ParseFloat(config["dynamic."+platform+".memory"], 64)
	if err != nil {
		mem = 2
	}
	cores, err := strconv.ParseFloat(config["dynamic."+platform+".cores"], 64)
	if err != nil {
		cores = 0.25
	}
	volumeSize, err := strconv.ParseFloat(config["dynamic."+platform+".disk"], 64)
	// IBM docs says it is potentially unwanted to downsize the bootable volume
	if err != nil || volumeSize < 100 {
		volumeSize = 100
	}

	userDataString := config["dynamic."+platform+".user-data"]
	var base64userData = ""
	if userDataString != "" {
		base64userData = base64.StdEncoding.EncodeToString([]byte(userDataString))
	}

	return IBMPowerDynamicConfig{
		Key:             config["dynamic."+platform+".key"],
		ImageId:         config["dynamic."+platform+".image"],
		Secret:          config["dynamic."+platform+".secret"],
		Url:             config["dynamic."+platform+".url"],
		CRN:             config["dynamic."+platform+".crn"],
		Network:         config["dynamic."+platform+".network"],
		System:          config["dynamic."+platform+".system"],
		Cores:           cores,
		Memory:          mem,
		Disk:            volumeSize,
		SystemNamespace: systemNamespace,
		UserData:        base64userData,
		ProcType:        "shared",
	}
}

// LaunchInstance creates a Power Systems VM instance on the pw cloud and returns its identifier. This function
// is implemented as part of the CloudProvider interface, which is why some of the arguments are unused for this
// particular implementation.
func (pw IBMPowerDynamicConfig) LaunchInstance(kubeClient client.Client, ctx context.Context, taskRunName string, instanceTag string, _ map[string]string) (cloud.InstanceIdentifier, error) {
	log := logr.FromContextOrDiscard(ctx)
	service, err := pw.createAuthenticatedBaseService(ctx, kubeClient)
	if err != nil {
		return "", fmt.Errorf("failed to create an authenticated base service: %w", err)
	}

	instanceName, err := createInstanceName(instanceTag)
	if err != nil {
		return "", fmt.Errorf("failed to create an instance name: %w", err)
	}
	// workaround to avoid BadRequest-s, after config validation introduced that might be not an issue anymore
	if len(instanceName) > maxPPCNameLength {
		log.Info("WARN: generated instance name is too long. Instance tag need to be shortened. Truncating to the max possible length.", "tag", instanceTag)
		instanceName = instanceName[:maxPPCNameLength]
	}

	instance, err := pw.launchInstance(ctx, service, instanceName)
	if err != nil {
		err = fmt.Errorf("failed to create a Power Systems instance: %w", err)
	}
	return instance, err

}

// CountInstances returns the number of Power Systems VM instances on the pw cloud whose names start
// with instanceTag.
func (pw IBMPowerDynamicConfig) CountInstances(kubeClient client.Client, ctx context.Context, instanceTag string) (int, error) {
	service, err := pw.createAuthenticatedBaseService(ctx, kubeClient)
	if err != nil {
		return -1, fmt.Errorf("failed to create an authenticated base service: %w", err)
	}

	instances, err := pw.listInstances(ctx, service)
	if err != nil {
		return -1, fmt.Errorf("failed to fetch Power Systems instances: %w", err)
	}

	count := 0
	for _, instance := range instances.PvmInstances {
		if strings.HasPrefix(*instance.ServerName, instanceTag) {
			count++
		}
	}
	return count, nil
}

// GetInstanceAddress returns the IP Address associated with the instanceID Power Systems VM instance.
func (pw IBMPowerDynamicConfig) GetInstanceAddress(kubeClient client.Client, ctx context.Context, instanceId cloud.InstanceIdentifier) (string, error) {
	log := logr.FromContextOrDiscard(ctx)
	service, err := pw.createAuthenticatedBaseService(ctx, kubeClient)
	if err != nil {
		return "", fmt.Errorf("failed to create an authenticated base service: %w", err)
	}

	// Errors regarding finding the instance, getting it's IP address and checking if the
	// address is live are not returned as we are waiting for the network interface to start up.
	// This is a normal part of the instance allocation process.
	instance, err := pw.getInstance(ctx, service, string(instanceId))
	if err != nil {
		log.Error(err, "failed to get instance", "instanceId", instanceId)
		return "", nil
	}
	ip, err := retrieveInstanceIp(*instance.PvmInstanceID, instance.Networks)
	if err != nil {
		log.Error(err, "failed to retrieve IP address", "instanceId", instanceId)
		return "", nil
	}
	//Don't return an error here since an IP address can take a while to become "live"
	if err = checkIfIpIsLive(ctx, ip); err != nil {
		log.Error(
			err,
			"failed to check if IP address was live",
			"instanceId", instanceId,
			"ip", ip,
		)
		return "", nil
	}
	return ip, nil
}

// ListInstances returns a collection of accessible Power Systems VM instances, on the pw cloud,
// whose names start with instanceTag.
func (pw IBMPowerDynamicConfig) ListInstances(kubeClient client.Client, ctx context.Context, instanceTag string) ([]cloud.CloudVMInstance, error) {
	log := logr.FromContextOrDiscard(ctx)
	log.Info("Listing Power Systems instances", "tag", instanceTag)
	service, err := pw.createAuthenticatedBaseService(ctx, kubeClient)
	if err != nil {
		return nil, fmt.Errorf("failed to create an authenticated base service: %w", err)
	}

	pvmInstancesCollection, err := pw.listInstances(ctx, service)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch Power Systems instances: %w", err)
	}

	vmInstances := make([]cloud.CloudVMInstance, 0, len(pvmInstancesCollection.PvmInstances))
	// Ensure all listed instances have a reachable IP address
	for _, instance := range pvmInstancesCollection.PvmInstances {
		if !strings.HasPrefix(*instance.ServerName, instanceTag) {
			continue
		}
		identifier := cloud.InstanceIdentifier(*instance.PvmInstanceID)
		createdAt := time.Time(instance.CreationDate)
		ip, err := retrieveInstanceIp(*instance.PvmInstanceID, instance.Networks)
		if err != nil {
			log.Error(err, "not listing instance as IP address cannot be assigned yet", "instance", identifier)
			continue
		}
		if err = checkIfIpIsLive(ctx, ip); err != nil {
			log.Error(
				err,
				"not listing instance as IP address cannot be accessed yet",
				"instanceId", identifier,
				"ip", ip,
			)
			continue
		}
		newVmInstance := cloud.CloudVMInstance{InstanceId: identifier, Address: ip, StartTime: createdAt}
		vmInstances = append(vmInstances, newVmInstance)

	}
	log.Info("Finished listing Power Systems instances.", "count", len(vmInstances))
	return vmInstances, nil
}

// TerminateInstance tries to delete a specific Power Systems VM instance on the pw cloud for 10 minutes
// or until the instance is deleted.
func (pw IBMPowerDynamicConfig) TerminateInstance(kubeClient client.Client, ctx context.Context, instanceId cloud.InstanceIdentifier) error {
	log := logr.FromContextOrDiscard(ctx)
	log.Info("attempting to terminate power server", "instance", instanceId)
	service, err := pw.createAuthenticatedBaseService(ctx, kubeClient)
	if err != nil {
		return fmt.Errorf("failed to create an authenticated base service: %w", err)
	}

	_ = pw.deleteInstance(ctx, service, string(instanceId))

	// Iterate for 10 minutes
	timeout := time.Now().Add(time.Minute * 10)
	go func() {
		localCtx := context.WithoutCancel(ctx)
		service, err := pw.createAuthenticatedBaseService(localCtx, kubeClient)
		if err != nil {
			return
		}

		for {
			_, err := pw.getInstance(localCtx, service, string(instanceId))
			// Instance has already been deleted
			if err != nil {
				return
			}
			//TODO: clarify comment ->we want to make really sure it is gone, delete opts don't
			// really work when the server is starting so we just try in a loop
			err = pw.deleteInstance(localCtx, service, string(instanceId))
			if err != nil {
				log.Error(err, "failed to delete Power System instance")
			}
			if timeout.Before(time.Now()) {
				return
			}

			// Sleep 10 seconds between each execution
			time.Sleep(time.Second * 10)
		}
	}()
	return nil
}

// GetState returns ibmp's VM state from the IBM Power Systems Virtual Server service.
// See https://cloud.ibm.com/apidocs/power-cloud#pcloud-pvminstances-get for more information.
func (ibmp IBMPowerDynamicConfig) GetState(kubeClient client.Client, ctx context.Context, instanceId cloud.InstanceIdentifier) (string, error) {
	service, err := ibmp.createAuthenticatedBaseService(ctx, kubeClient)
	if err != nil {
		return "", fmt.Errorf("failed to create an authenticated base service: %w", err)
	}

	instance, err := ibmp.getInstance(ctx, service, string(instanceId))
	// Probably still waiting for the instance to come up
	if err != nil {
		return "", nil
	}

	// An instance in a failed state has a status of "ERROR" and a health of "CRITICAL"
	if *instance.Status == "ERROR" && instance.Health.Status == "CRITICAL" {
		return "FAILED", nil
	}
	return "OK", nil
}

func (pw IBMPowerDynamicConfig) SshUser() string {
	return "root"
}

// An IBMPowerDynamicConfig represents a configuration for an IBM Power Systems cloud instance.
// The zero value (where each field will be assigned its type's zero value) is not a
// valid IBMPowerDynamicConfig.
type IBMPowerDynamicConfig struct {
	// SystemNamespace is the name of the Kubernetes namespace where the specified
	// secrets are stored.
	SystemNamespace string

	// Secret is the name of the Kubernetes ExternalSecret resource to use to
	// connect and authenticate with the IBM cloud service.
	Secret string

	// Key is the name of the public SSH key to be used when creating the instance.
	Key string

	// ImageId is the image to use when creating the instance.
	ImageId string

	// Url is the url to use when creating the base service for the instance.
	Url string

	// CRN is the Cloud Resource Name used to uniquely identify the cloud the instance
	// is hosted on.
	CRN string

	// Network is the network ID to use when creating the instance.
	Network string

	// Cores is the number of computer cores to allocate for the instance.
	Cores float64

	// Memory is the amount of memory (in GB) allocated to the instance.
	Memory float64

	// Disk is the amount of permanent storage (in GB) allocated to the instance.
	Disk float64

	// System is the type of system to start in the instance.
	System string

	// TODO: determine what this is for (see commonUserData in ibmp_test.go)
	UserData string

	// ProcessorType is the processor type to be used in the instance.
	// Possible values are "dedicated", "shared", and "capped".
	ProcType string
}
