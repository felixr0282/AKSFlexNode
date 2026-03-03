package v20260301

import (
	"bytes"
	"context"
	"errors"

	"github.com/Azure/AKSFlexNode/components/kubelet"
	"github.com/Azure/AKSFlexNode/pkg/config"
	"github.com/Azure/AKSFlexNode/pkg/systemd"
)

func (s *startKubeletServiceAction) ensureSystemdUnit(
	ctx context.Context,
	needsRestart bool,
	spec *kubelet.StartKubeletServiceSpec,
) error {
	kubeletConfig := spec.GetKubeletConfig()

	var (
		useBootstrapKubeconfig bool
		rotateCertificates     bool
	)
	if spec.GetNodeAuthInfo().HasBootstrapTokenCredential() {
		useBootstrapKubeconfig = true
		// When bootstrap token is used, kubelet client certificate is rotated by kubelet itself
		// TODO: consider making this configurable in the spec level
		rotateCertificates = true
	}

	params := map[string]any{
		"NodeLabels":              mapPairsToString(spec.GetNodeLabels(), "=", ","),
		"Verbosity":               kubeletConfig.GetVerbosity(),
		"ClientCAFile":            apiServerClientCAPath, // prepared in ensureAPIServerCA
		"ClusterDNS":              kubeletConfig.GetClusterDns(),
		"EvictionHard":            mapPairsToString(kubeletConfig.GetEvictionHard(), "<", ","),
		"KubeReserved":            mapPairsToString(kubeletConfig.GetKubeReserved(), "=", ","),
		"ImageGCHighThreshold":    kubeletConfig.GetImageGcHighThreshold(),
		"ImageGCLowThreshold":     kubeletConfig.GetImageGcLowThreshold(),
		"MaxPods":                 kubeletConfig.GetMaxPods(),
		"RotateCertificates":      rotateCertificates,
		"UseBootstrapKubeconfig":  useBootstrapKubeconfig,
		"BootstrapKubeconfigPath": config.KubeletBootstrapKubeconfigPath,
		"KubeconfigPath":          config.KubeletKubeconfigPath,
	}

	b := &bytes.Buffer{}
	if err := assetsTemplate.ExecuteTemplate(b, "kubelet.service", params); err != nil {
		return err
	}

	unitUpdated, err := s.systemd.EnsureUnitFile(ctx, systemdUnitKubelet, b.Bytes())
	if err != nil {
		return err
	}

	if unitUpdated {
		if err := s.systemd.DaemonReload(ctx); err != nil {
			return err
		}
	}

	needsRestart = needsRestart || unitUpdated

	status, err := s.systemd.GetUnitStatus(ctx, systemdUnitKubelet)
	switch {
	case errors.Is(err, systemd.ErrUnitNotFound):
		return s.systemd.StartUnit(ctx, systemdUnitKubelet)
	case err != nil:
		return err
	default:
		if status.ActiveState != systemd.UnitActiveStateActive {
			return s.systemd.StartUnit(ctx, systemdUnitKubelet)
		}
		if needsRestart {
			return s.systemd.ReloadOrRestartUnit(ctx, systemdUnitKubelet)
		}
		return nil
	}
}
