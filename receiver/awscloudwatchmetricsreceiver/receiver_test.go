// Copyright The OpenTelemetry Authors
// SPDX-License-Identifier: Apache-2.0

package awscloudwatchmetricsreceiver // import "github.com/open-telemetry/opentelemetry-collector-contrib/receiver/awscloudwatchmetricsreceiver"

import (
	"context"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/cloudwatch"
	"github.com/aws/aws-sdk-go-v2/service/cloudwatch/types"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/collector/component/componenttest"
	"go.opentelemetry.io/collector/consumer/consumertest"
	"go.uber.org/zap"
)

const (
	namespace  = "AWS/EC2"
	metricname = "CPUUtilization"
	agg        = "Average"
	DimName    = "InstanceId"
	DimValue   = "i-1234567890abcdef0"
)

func TestDefaultFactory(t *testing.T) {
	cfg := createDefaultConfig().(*Config)
	cfg.Region = "eu-west-1"

	sink := &consumertest.MetricsSink{}
	mtrcRcvr := newMetricReceiver(cfg, zap.NewNop(), sink)

	err := mtrcRcvr.Start(context.Background(), componenttest.NewNopHost())
	require.NoError(t, err)

	err = mtrcRcvr.Shutdown(context.Background())
	require.NoError(t, err)
}

func TestGroupConfig(t *testing.T) {
	cfg := createDefaultConfig().(*Config)
	cfg.Region = "eu-west-1"
	cfg.PollInterval = time.Second * 1
	cfg.Metrics = &MetricsConfig{
		Group: []GroupConfig{
			{
				Namespace: namespace,
				Period:    time.Second * 60 * 5,
				MetricName: []NamedConfig{
					{
						MetricName:     metricname,
						AwsAggregation: agg,
						Dimensions: []MetricDimensionsConfig{
							{
								Name:  DimName,
								Value: DimValue,
							},
						},
					},
				},
			},
		},
	}
	sink := &consumertest.MetricsSink{}
	mtrcRcvr := newMetricReceiver(cfg, zap.NewNop(), sink)
	mtrcRcvr.client = defaultMockCloudWatchClient()

	err := mtrcRcvr.Start(context.Background(), componenttest.NewNopHost())
	require.NoError(t, err)

	require.Eventually(t, func() bool {
		return sink.DataPointCount() > 0
	}, 2000*time.Second, 10*time.Millisecond)

	err = mtrcRcvr.Shutdown(context.Background())
	require.NoError(t, err)
}

const (
	testNamespace      = "EC2"
	testMetricName     = "CPUUtilization"
	TestDimensionName  = "InstanceId"
	TestDimensionValue = "i-1234567890abcdef0"
)

var testDimensions = []types.Dimension{
	{
		Name:  aws.String(TestDimensionName),
		Value: aws.String(TestDimensionValue),
	},
}

func defaultMockCloudWatchClient() client {
	mc := &MockClient{}

	mc.On("ListMetrics", mock.Anything, mock.Anything, mock.Anything).Return(
		&cloudwatch.ListMetricsOutput{
			Metrics: []types.Metric{
				{
					MetricName: aws.String(testMetricName),
					Namespace:  aws.String(testNamespace),
					Dimensions: testDimensions,
				},
			},
			NextToken: nil,
		}, nil)

	mc.On("GetMetricData", mock.Anything, mock.Anything, mock.Anything).Return(
		&cloudwatch.GetMetricDataOutput{
			MetricDataResults: []types.MetricDataResult{
				{
					Id:         aws.String("t1"),
					Label:      aws.String("testLabel"),
					Values:     []float64{1.0},
					Timestamps: []time.Time{time.Now()},
					StatusCode: types.StatusCodeComplete,
				},
			},
			NextToken: nil,
		}, nil)
	return mc
}

type MockClient struct {
	mock.Mock
}

func (m *MockClient) GetMetricData(ctx context.Context, params *cloudwatch.GetMetricDataInput, optFns ...func(*cloudwatch.Options)) (*cloudwatch.GetMetricDataOutput, error) {
	args := m.Called(ctx, params, optFns)
	return args.Get(0).(*cloudwatch.GetMetricDataOutput), args.Error(1)
}

func (m *MockClient) ListMetrics(ctx context.Context, params *cloudwatch.ListMetricsInput, optFns ...func(*cloudwatch.Options)) (*cloudwatch.ListMetricsOutput, error) {
	args := m.Called(ctx, params, optFns)
	return args.Get(0).(*cloudwatch.ListMetricsOutput), args.Error(1)
}
