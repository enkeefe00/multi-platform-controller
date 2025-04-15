package aws

import (
	"github.com/go-logr/logr"
	"github.com/konflux-ci/multi-platform-controller/pkg/cloud"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
	"github.com/aws/aws-sdk-go-v2/service/ec2/types"
)

var _ = Describe("AWS EC2 Helper Functions", func() {
	// This test is only here to check AWS connectivity in a very primitive and quick way until KFLUXINFRA-1065
	// work starts
	Describe("Ping IP Address", func() {
		DescribeTable("Testing the ability to ping a remote AWS ec2 instance via SSH",
			func(testInstanceIP string, shouldFail bool) {

				err := pingIPAddress(testInstanceIP)

				if !shouldFail {
					Expect(err).Should(BeNil())
				} else {
					Expect(err).Should(HaveOccurred())
				}

			},
			Entry("Positive test - IP address", "150.239.19.36", false),
			Entry("Negative test - no such IP address", "192.168.4.231", true),
			Entry("Negative test - no such DNS name", "not a DNS name, that's for sure", true),
			Entry("Negative test - not an IP address", "Not an IP address", true),
		)
	})

	DescribeTable("Find VM instances linked to non-existent TaskRuns",
		func(log logr.Logger, ec2Reservations []types.Reservation, existingTaskRuns map[string][]string, expectedInstances []string) {
			cfg := AWSEc2DynamicConfig{}
			Expect(cfg.findInstancesWithoutTaskRuns(log, ec2Reservations, existingTaskRuns)).To(Equal(expectedInstances))
		},
		Entry("no reservations", logr.Discard(),
			[]types.Reservation{}, map[string][]string{},
			nil,
		),
		Entry("no instances", logr.Discard(),
			[]types.Reservation{{Instances: []types.Instance{}}},
			map[string][]string{},
			nil,
		),
		Entry("instance w/ no tags", logr.Discard(),
			[]types.Reservation{
				{Instances: []types.Instance{
					{InstanceId: aws.String("id"), Tags: []types.Tag{}},
				}},
			},
			map[string][]string{},
			[]string{"id"},
		),
		Entry("instance w/ no TaskRun ID tag", logr.Discard(),
			[]types.Reservation{
				{Instances: []types.Instance{
					{
						InstanceId: aws.String("id"),
						Tags:       []types.Tag{{Key: aws.String("key"), Value: aws.String("value")}},
					},
				}},
			},
			map[string][]string{},
			[]string{"id"},
		),
		Entry("instance w/ invalid TaskRun ID", logr.Discard(),
			[]types.Reservation{
				{Instances: []types.Instance{
					{
						InstanceId: aws.String("id"),
						Tags:       []types.Tag{{Key: aws.String(cloud.TaskRunTagKey), Value: aws.String("value")}},
					},
				}},
			},
			map[string][]string{},
			[]string{"id"},
		),
		Entry("all instances have existing TaskRuns", logr.Discard(),
			[]types.Reservation{
				{Instances: []types.Instance{
					{
						InstanceId: aws.String("task1"),
						Tags:       []types.Tag{{Key: aws.String(cloud.TaskRunTagKey), Value: aws.String("test:task1")}},
					},
					{
						InstanceId: aws.String("task2"),
						Tags:       []types.Tag{{Key: aws.String(cloud.TaskRunTagKey), Value: aws.String("test:task2")}},
					},
					{
						InstanceId: aws.String("task3"),
						Tags:       []types.Tag{{Key: aws.String(cloud.TaskRunTagKey), Value: aws.String("test:task3")}},
					},
					{
						InstanceId: aws.String("task4"),
						Tags:       []types.Tag{{Key: aws.String(cloud.TaskRunTagKey), Value: aws.String("test:task4")}},
					},
				}},
			},
			map[string][]string{"test": {"task1", "task2", "task3", "task4"}},
			nil,
		),
		Entry("one instance doesn't have a TaskRun", logr.Discard(),
			[]types.Reservation{
				{Instances: []types.Instance{
					{
						InstanceId: aws.String("task-a"),
						Tags:       []types.Tag{{Key: aws.String(cloud.TaskRunTagKey), Value: aws.String("test:task-a")}},
					},
					{
						InstanceId: aws.String("task2"),
						Tags:       []types.Tag{{Key: aws.String(cloud.TaskRunTagKey), Value: aws.String("test:task2")}},
					},
					{
						InstanceId: aws.String("task3"),
						Tags:       []types.Tag{{Key: aws.String(cloud.TaskRunTagKey), Value: aws.String("test:task3")}},
					},
					{
						InstanceId: aws.String("task4"),
						Tags:       []types.Tag{{Key: aws.String(cloud.TaskRunTagKey), Value: aws.String("test:task4")}},
					},
				}},
			},
			map[string][]string{"test": {"task1", "task2", "task3", "task4"}},
			[]string{"task-a"},
		),
		Entry("multiple instances don't have a TaskRun", logr.Discard(),
			[]types.Reservation{
				{Instances: []types.Instance{
					{
						InstanceId: aws.String("task-a"),
						Tags:       []types.Tag{{Key: aws.String(cloud.TaskRunTagKey), Value: aws.String("test:task-a")}},
					},
					{
						InstanceId: aws.String("task-b"),
						Tags:       []types.Tag{{Key: aws.String(cloud.TaskRunTagKey), Value: aws.String("")}},
					},
					{
						InstanceId: aws.String("task3"),
						Tags:       []types.Tag{{Key: aws.String(cloud.TaskRunTagKey), Value: aws.String("test:task3")}},
					},
					{
						InstanceId: aws.String("task4"),
						Tags:       []types.Tag{{Key: aws.String(cloud.TaskRunTagKey), Value: aws.String("test:task4")}},
					},
				}},
			},
			map[string][]string{"test": {"task1", "task2", "task3", "task4"}},
			[]string{"task-a", "task-b"}),
		Entry("no instances have a TaskRun", logr.Discard(),
			[]types.Reservation{
				{Instances: []types.Instance{
					{
						InstanceId: aws.String("task1"),
						Tags:       []types.Tag{{Key: aws.String(cloud.TaskRunTagKey), Value: aws.String("test:task1")}},
					},
					{
						InstanceId: aws.String("task2"),
						Tags:       []types.Tag{{Key: aws.String(cloud.TaskRunTagKey), Value: aws.String("test:task2")}},
					},
					{
						InstanceId: aws.String("task3"),
						Tags:       []types.Tag{{Key: aws.String(cloud.TaskRunTagKey), Value: aws.String("test:task3")}},
					},
					{
						InstanceId: aws.String("task4"),
						Tags:       []types.Tag{{Key: aws.String(cloud.TaskRunTagKey), Value: aws.String("test:task4")}},
					},
				}},
			},
			map[string][]string{"test-namespace": {"task1", "task2", "task3", "task4"}},
			[]string{"task1", "task2", "task3", "task4"}),
	)

	DescribeTable("Configure instance",
		func(taskRunName string, instanceTag string, additionalTags map[string]string, expectedSecurityGroups []string,
			expectedSecurityGroupIds []string, expectedInstanceProfileName *string, expectedInstanceProfileARN *string,
			expectedSubnetId *string, expectedMaxSpotPrice string, expectedTags []types.Tag, ec2Config AWSEc2DynamicConfig) {

			expectedConfig := ec2.RunInstancesInput{
				KeyName:          aws.String(ec2Config.KeyName),
				ImageId:          aws.String(ec2Config.Ami),
				InstanceType:     types.InstanceType(ec2Config.InstanceType),
				MinCount:         aws.Int32(int32(1)),
				MaxCount:         aws.Int32(int32(1)),
				EbsOptimized:     aws.Bool(true),
				SecurityGroups:   expectedSecurityGroups,
				SecurityGroupIds: expectedSecurityGroupIds,
				IamInstanceProfile: &types.IamInstanceProfileSpecification{
					Name: expectedInstanceProfileName,
					Arn:  expectedInstanceProfileARN,
				},
				SubnetId: expectedSubnetId,
				UserData: ec2Config.UserData,
				BlockDeviceMappings: []types.BlockDeviceMapping{{
					DeviceName:  aws.String("/dev/sda1"),
					VirtualName: aws.String("ephemeral0"),
					Ebs: &types.EbsBlockDevice{
						DeleteOnTermination: aws.Bool(true),
						VolumeSize:          aws.Int32(ec2Config.Disk),
						VolumeType:          types.VolumeTypeGp3,
						Iops:                ec2Config.Iops,
						Throughput:          ec2Config.Throughput,
					},
				}},
				InstanceInitiatedShutdownBehavior: types.ShutdownBehaviorTerminate,
				TagSpecifications: []types.TagSpecification{
					{ResourceType: types.ResourceTypeInstance, Tags: expectedTags},
				},
			}
			if expectedMaxSpotPrice != "" {
				expectedConfig.InstanceMarketOptions = &types.InstanceMarketOptionsRequest{
					MarketType: types.MarketTypeSpot,
					SpotOptions: &types.SpotMarketOptions{
						MaxPrice:                     &expectedMaxSpotPrice,
						InstanceInterruptionBehavior: types.InstanceInterruptionBehaviorTerminate,
						SpotInstanceType:             types.SpotInstanceTypeOneTime,
					},
				}
			}
			if expectedInstanceProfileARN == nil && expectedInstanceProfileName == nil {
				expectedConfig.IamInstanceProfile = nil
			}

			actualConfig := *ec2Config.configureInstance(taskRunName, instanceTag, additionalTags)
			Expect(actualConfig).To(Equal(expectedConfig))

		},
		Entry("empty fields", "test task", "prod-s390x", map[string]string{"extra tag": "extra"}, nil,
			nil, nil, nil, nil, "",
			[]types.Tag{
				{Key: aws.String(MultiPlatformManaged), Value: aws.String("true")},
				{Key: aws.String(cloud.InstanceTag), Value: aws.String("prod-s390x")},
				{Key: aws.String("Name"), Value: aws.String("multi-platform-builder-test task")},
				{Key: aws.String("extra tag"), Value: aws.String("extra")},
			},
			AWSEc2DynamicConfig{},
		),
		Entry("all fields configured", "test task", "prod-s390x", map[string]string{"extra tag": "extra"}, []string{"test-group"},
			[]string{"test-id"}, aws.String("test-name"), aws.String("test-arn"), aws.String("test-subnet"), "test-price",
			[]types.Tag{
				{Key: aws.String(MultiPlatformManaged), Value: aws.String("true")},
				{Key: aws.String(cloud.InstanceTag), Value: aws.String("prod-s390x")},
				{Key: aws.String("Name"), Value: aws.String("multi-platform-builder-test task")},
				{Key: aws.String("extra tag"), Value: aws.String("extra")},
			},
			AWSEc2DynamicConfig{
				Region:               "test-region",
				Ami:                  "test-ami",
				InstanceType:         "test-type",
				KeyName:              "test-key",
				Secret:               "test-secret",
				SystemNamespace:      "test-namespace",
				SecurityGroup:        "test-group",
				SecurityGroupId:      "test-id",
				SubnetId:             "test-subnet",
				Disk:                 1,
				MaxSpotInstancePrice: "test-price",
				InstanceProfileName:  "test-name",
				InstanceProfileArn:   "test-arn",
				Iops:                 aws.Int32(1),
				Throughput:           aws.Int32(1),
				UserData:             aws.String("test-data"),
			},
		),
		Entry("empty TaskRun name", "", "prod-s390x", map[string]string{"extra tag": "extra"}, []string{"test-group"},
			[]string{"test-id"}, nil, aws.String("test-arn"), aws.String("test-subnet"), "test-price",
			[]types.Tag{
				{Key: aws.String(MultiPlatformManaged), Value: aws.String("true")},
				{Key: aws.String(cloud.InstanceTag), Value: aws.String("prod-s390x")},
				{Key: aws.String("Name"), Value: aws.String("multi-platform-builder-")},
				{Key: aws.String("extra tag"), Value: aws.String("extra")},
			},
			AWSEc2DynamicConfig{
				Region:               "test-region",
				Ami:                  "test-ami",
				InstanceType:         "test-type",
				KeyName:              "test-key",
				Secret:               "test-secret",
				SystemNamespace:      "test-namespace",
				SecurityGroup:        "test-group",
				SecurityGroupId:      "test-id",
				SubnetId:             "test-subnet",
				Disk:                 1,
				MaxSpotInstancePrice: "test-price",
				InstanceProfileArn:   "test-arn",
				Iops:                 aws.Int32(1),
				Throughput:           aws.Int32(1),
				UserData:             aws.String("test-data"),
			},
		),
		Entry("empty instance tag", "test task", "", map[string]string{"extra tag": "extra"}, []string{"test-group"},
			[]string{"test-id"}, aws.String("test-name"), nil, aws.String("test-subnet"), "test-price",
			[]types.Tag{
				{Key: aws.String(MultiPlatformManaged), Value: aws.String("true")},
				{Key: aws.String(cloud.InstanceTag), Value: aws.String("")},
				{Key: aws.String("Name"), Value: aws.String("multi-platform-builder-test task")},
				{Key: aws.String("extra tag"), Value: aws.String("extra")},
			},
			AWSEc2DynamicConfig{
				Region:               "test-region",
				Ami:                  "test-ami",
				InstanceType:         "test-type",
				KeyName:              "test-key",
				Secret:               "test-secret",
				SystemNamespace:      "test-namespace",
				SecurityGroup:        "test-group",
				SecurityGroupId:      "test-id",
				SubnetId:             "test-subnet",
				Disk:                 1,
				MaxSpotInstancePrice: "test-price",
				InstanceProfileName:  "test-name",
				Iops:                 aws.Int32(1),
				Throughput:           aws.Int32(1),
				UserData:             aws.String("test-data"),
			},
		),
		Entry("no additional tags", "test task", "prod-s390x", nil, nil,
			[]string{"test-id"}, aws.String("test-name"), aws.String("test-arn"), aws.String("test-subnet"), "test-price",
			[]types.Tag{
				{Key: aws.String(MultiPlatformManaged), Value: aws.String("true")},
				{Key: aws.String(cloud.InstanceTag), Value: aws.String("prod-s390x")},
				{Key: aws.String("Name"), Value: aws.String("multi-platform-builder-test task")},
			},
			AWSEc2DynamicConfig{
				Region:               "test-region",
				Ami:                  "test-ami",
				InstanceType:         "test-type",
				KeyName:              "test-key",
				Secret:               "test-secret",
				SecurityGroupId:      "test-id",
				SubnetId:             "test-subnet",
				Disk:                 1,
				MaxSpotInstancePrice: "test-price",
				InstanceProfileName:  "test-name",
				InstanceProfileArn:   "test-arn",
				Iops:                 aws.Int32(1),
				Throughput:           aws.Int32(1),
				UserData:             aws.String("test-data"),
			},
		),
	)
})
