// Feature flags for the Ubuntu Auto-Update dashboard.
//
// The UI reads feature availability from the backend's /api/v1/license
// endpoint. Paid features show a locked state with an upgrade tooltip
// when the license doesn't include them.
//
// Usage:
//   import { useFeatures } from './featureFlags';
//   const { hasFeature, isLoading } = useFeatures();
//   if (hasFeature('advanced_viz')) { ... }

import { useState, useEffect, useCallback, createContext, useContext } from 'react';
import { apiGet } from './api';

// Mirrors backend/pkg/licensing Feature constants.
export type Feature =
  | 'advanced_viz'
  | 'collaboration'
  | 'full_rbac'
  | 'sso_oidc'
  | 'extended_audit'
  | 'managed_hosting'
  | 'enterprise_sla';

export interface LicenseInfo {
  valid: boolean;
  expires_at?: string;
  max_hosts?: number;
  features: Feature[];
  license_id?: string;
  error?: string;
}

// Free-tier features available without a license.
const FREE_FEATURES: Feature[] = [];

interface FeatureState {
  license: LicenseInfo | null;
  isLoading: boolean;
  error: string | null;
  hasFeature: (f: Feature) => boolean;
  refresh: () => Promise<void>;
}

const FeatureContext = createContext<FeatureState>({
  license: null,
  isLoading: true,
  error: null,
  hasFeature: () => false,
  refresh: async () => {},
});

export function useFeatures(): FeatureState {
  return useContext(FeatureContext);
}

export function FeatureProvider({ children }: { children: React.ReactNode }) {
  const [license, setLicense] = useState<LicenseInfo | null>(null);
  const [isLoading, setIsLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);

  const refresh = useCallback(async () => {
    try {
      setIsLoading(true);
      const data = await apiGet<LicenseInfo>('/api/v1/license');
      setLicense(data);
      setError(null);
    } catch {
      // Backend might not have the endpoint yet — degrade gracefully.
      setLicense({ valid: false, features: [], error: 'Unavailable' });
    } finally {
      setIsLoading(false);
    }
  }, []);

  useEffect(() => {
    refresh();
  }, [refresh]);

  const hasFeature = useCallback(
    (f: Feature): boolean => {
      if (FREE_FEATURES.includes(f)) return true;
      if (!license?.valid) return false;
      return license.features.includes(f);
    },
    [license],
  );

  return (
    <FeatureContext.Provider value={{ license, isLoading, error, hasFeature, refresh }}>
      {children}
    </FeatureContext.Provider>
  );
}

// ── UI helpers ──────────────────────────────────────────────────────────────

/**
 * Tooltip text shown on locked UI elements.
 */
export function upgradeTooltip(feature: Feature): string {
  const names: Record<Feature, string> = {
    advanced_viz: 'Advanced Data Visualization',
    collaboration: 'Shared Dashboards & Comments',
    full_rbac: 'Fine-Grained RBAC',
    sso_oidc: 'SSO / OIDC Integration',
    extended_audit: 'Extended Audit Log Retention',
    managed_hosting: 'Managed Hosting',
    enterprise_sla: 'Enterprise SLA & Support',
  };
  return `${names[feature] || feature} requires a paid license. Contact sales to upgrade.`;
}
