# Microsoft Azure Architecture Center, Cloud Design Patterns, "Health Endpoint Monitoring pattern"

**Citation**: Microsoft Azure Architecture Center, Cloud Design Patterns, "Health Endpoint Monitoring pattern".

**Opened**: https://learn.microsoft.com/azure/architecture/patterns/health-endpoint-monitoring (opened 2026-11-13 by the lifecycle builder).

**Used for**: The liveness/readiness seam of lifecycle: checks run under a deadline and aggregated; readiness false while starting and from the instant shutdown begins (the Kubernetes SIGTERM convention is named only as convention). httpx.Health (wave 9) exposes it.

**Used by (IDs)**: TB-LIFE-012, TB-LIFE-013, TB-LIFE-014.
