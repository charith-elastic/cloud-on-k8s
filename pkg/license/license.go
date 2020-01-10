// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package license

import (
	"encoding/json"
	"fmt"
	"math"
	"strconv"
	"time"

	"github.com/elastic/cloud-on-k8s/pkg/controller/common"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/license"
	"github.com/elastic/cloud-on-k8s/pkg/utils/k8s"
	"github.com/prometheus/client_golang/prometheus"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	crmetrics "sigs.k8s.io/controller-runtime/pkg/metrics"
)

const (
	// defaultOperatorLicenseLevel is the default license level when no operator license is installed
	defaultOperatorLicenseLevel = "basic"
	// licensingCfgMapName is the name of the config map used to store licensing information
	licensingCfgMapName = "elastic-licensing"
	// Type represents the Elastic usage type used to mark the config map that stores licensing information
	Type = "elastic-usage"

	// licenseLevelLabel is the Prometheus label describing the licence level.
	licenseLevelLabel = "license_level"
)

// LicensingInfo represents information about the operator license including the total memory of all Elastic managed
// components
type LicensingInfo struct {
	Timestamp                  string `json:"timestamp"`
	EckLicenseLevel            string `json:"eck_license_level"`
	TotalManagedMemory         string `json:"total_managed_memory"`
	MaxEnterpriseResourceUnits string `json:"max_enterprise_resource_units,omitempty"`
	EnterpriseResourceUnits    string `json:"enterprise_resource_units"`
}

// LicensingResolver resolves the licensing information of the operator
type LicensingResolver struct {
	operatorNS       string
	client           k8s.Client
	totalMemoryGauge *prometheus.GaugeVec
	eruGauge         *prometheus.GaugeVec
}

func NewLicensingResolver(operatorNS string, client k8s.Client) *LicensingResolver {
	totalMemoryGauge := registerGauge(prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Namespace: "elastic",
		Subsystem: "licensing",
		Name:      "memory_gigabytes_total",
		Help:      "Total memory used in GB",
	}, []string{licenseLevelLabel}))

	eruGauge := registerGauge(prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Namespace: "elastic",
		Subsystem: "licensing",
		Name:      "enterprise_resource_units_total",
		Help:      "Total enterprise resource units used",
	}, []string{licenseLevelLabel}))

	return &LicensingResolver{
		operatorNS:       operatorNS,
		client:           client,
		totalMemoryGauge: totalMemoryGauge,
		eruGauge:         eruGauge,
	}
}

func registerGauge(gauge *prometheus.GaugeVec) *prometheus.GaugeVec {
	err := crmetrics.Registry.Register(gauge)
	if err != nil {
		if existsErr, ok := err.(prometheus.AlreadyRegisteredError); ok {
			return existsErr.ExistingCollector.(*prometheus.GaugeVec)
		} else {
			panic(fmt.Errorf("failed to register licence information gauge: %w", err))
		}
	}

	return gauge
}

// ToInfo returns licensing information given the total memory of all Elastic managed components
func (r *LicensingResolver) ToInfo(totalMemory resource.Quantity) (LicensingInfo, error) {
	ERUs := inEnterpriseResourceUnits(totalMemory)
	memoryInGB := inGB(totalMemory)
	operatorLicense, err := r.getOperatorLicense()
	if err != nil {
		return LicensingInfo{}, err
	}

	licenseLevel := r.getOperatorLicenseLevel(operatorLicense)
	maxERUs := r.getMaxEnterpriseResourceUnits(operatorLicense)

	r.totalMemoryGauge.With(prometheus.Labels{licenseLevelLabel: licenseLevel}).Set(memoryInGB)
	r.eruGauge.With(prometheus.Labels{licenseLevelLabel: licenseLevel}).Set(float64(ERUs))

	licensingInfo := LicensingInfo{
		Timestamp:               time.Now().Format(time.RFC3339),
		EckLicenseLevel:         licenseLevel,
		TotalManagedMemory:      fmt.Sprintf("%0.2fGB", memoryInGB),
		EnterpriseResourceUnits: strconv.FormatInt(ERUs, 10),
	}

	// include the max ERUs only for a non trial/basic license
	if maxERUs > 0 {
		licensingInfo.MaxEnterpriseResourceUnits = strconv.Itoa(maxERUs)
	}

	return licensingInfo, nil
}

// Save updates or creates licensing information in a config map
func (r *LicensingResolver) Save(info LicensingInfo, operatorNs string) error {
	data, err := info.toMap()
	if err != nil {
		return err
	}

	log.V(1).Info("Saving", "namespace", operatorNs, "configmap_name", licensingCfgMapName, "license_info", info)
	cm := corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: operatorNs,
			Name:      licensingCfgMapName,
			Labels: map[string]string{
				common.TypeLabelName: Type,
			},
		},
		Data: data,
	}
	err = r.client.Update(&cm)
	if apierrors.IsNotFound(err) {
		return r.client.Create(&cm)
	}
	return err
}

// getOperatorLicense gets the operator license.
func (r *LicensingResolver) getOperatorLicense() (*license.EnterpriseLicense, error) {
	checker := license.NewLicenseChecker(r.client, r.operatorNS)
	return checker.CurrentEnterpriseLicense()
}

// getOperatorLicenseLevel gets the level of the operator license.
// If no license is given, the defaultOperatorLicenseLevel is returned.
func (r *LicensingResolver) getOperatorLicenseLevel(lic *license.EnterpriseLicense) string {
	if lic == nil {
		return defaultOperatorLicenseLevel
	}
	return string(lic.License.Type)
}

// getMaxEnterpriseResourceUnits returns the maximum of enterprise resources units that is allowed for a given license.
// For old style enterprise orchestration licenses which only have max_instances, the maximum of enterprise resources
// units is derived by dividing max_instances by 2.
func (r *LicensingResolver) getMaxEnterpriseResourceUnits(lic *license.EnterpriseLicense) int {
	if lic == nil {
		return 0
	}
	maxERUs := lic.License.MaxResourceUnits
	if maxERUs == 0 {
		maxERUs = lic.License.MaxInstances / 2
	}
	return maxERUs
}

// inGB converts a resource.Quantity in gigabytes
func inGB(q resource.Quantity) float64 {
	// divide the value (in bytes) per 1 billion (1GB)
	return float64(q.Value()) / 1000000000
}

// inEnterpriseResourceUnits converts a resource.Quantity to Elastic Enterprise resource units
func inEnterpriseResourceUnits(q resource.Quantity) int64 {
	// divide by the value (in bytes) per 64 billion (64 GB)
	eru := float64(q.Value()) / 64000000000
	// round to the nearest superior integer
	return int64(math.Ceil(eru))
}

// toMap transforms a LicensingInfo to a map of string, in order to fill in the data of a config map
func (i LicensingInfo) toMap() (map[string]string, error) {
	bytes, err := json.Marshal(&i)
	if err != nil {
		return nil, err
	}
	var m map[string]string
	err = json.Unmarshal(bytes, &m)
	if err != nil {
		return nil, err
	}
	return m, nil
}
