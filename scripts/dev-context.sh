#!/usr/bin/env bash
set -euo pipefail
ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"; cd "$ROOT_DIR"
usage(){ cat <<'EOF'
Usage: scripts/dev-context.sh {overview|channel-types|relay <provider>|controller-map|pricing|domain <name>|web [feature]}
EOF
}
require_file(){ [[ -f "$1" ]] || { printf 'MISSING %s\n' "$1" >&2; return 1; }; }
print_go_ranges(){ awk '
 function b(s){return gsub(/\{/,"",s)-gsub(/\}/,"",s)}
 /^func[[:space:]]/{if(n)printf "%d-%d %s\n",s,NR-1,n;s=NR;n=$0;d=0;a=1}
 a{d+=b($0);if(d<=0){printf "%d-%d %s\n",s,NR,n;n="";a=0}}
 END{if(n)printf "%d-%d %s\n",s,NR,n}' "$1"; }
print_ts_named_ranges(){ awk '
 /^  const ([A-Za-z0-9_]+(Fields|Section)|onSubmit)[[:space:]]*=/{if(n)printf "%d-%d %s\n",s,NR-1,n;s=NR;n=$0;d=0;a=1}
 a{d+=gsub(/\(/,"",$0)-gsub(/\)/,"",$0);if(d<=0&&index($0,"(")>0){printf "%d-%d %s\n",s,NR,n;n="";a=0}}
 END{if(n)printf "%d-%d %s\n",s,NR,n}' "$1"; }
overview(){
 printf '%s\n' '## Backend Go files by package'
  for p in router middleware controller service model setting common constant relay relaykit; do
  printf '\n### %s\n' "$p"; find "$p" -maxdepth 3 -type f -name '*.go' -print | sort
 done
 printf '%s\n' '## Frontend feature directories'; find web/src/features -mindepth 1 -maxdepth 1 -type d -print | sort
}
channel_types(){
 printf '%s\n' '## Channel/protocol symbols'; require_file constant/channel.go; require_file constant/api_type.go; require_file common/api_type.go
 rg -n '^[[:space:]]*ChannelType[A-Za-z0-9_]*([[:space:]]*=|[[:space:]]*$)|ChannelTypeNames|ChannelBaseURLs|GetChannelBaseURL|GetChannelTypeName|ChannelSpecialBases' constant/channel.go
 rg -n '^[[:space:]]*APIType[A-Za-z0-9_]*([[:space:]]*=|[[:space:]]*,|[[:space:]]*$)' constant/api_type.go
 rg -n 'RelayFormat[A-Za-z0-9_]*[[:space:]]*=' relaykit/types/relay_format.go
 rg -n 'ChannelType2APIType|SupportsResponsesCompact' common/api_type.go
 rg -n 'EndpointType[A-Za-z0-9_]*[[:space:]]*=' constant/endpoint_type.go common/endpoint_defaults.go
 rg -n 'GetEndpointTypesByChannelType|GetModelSupportEndpointTypes|EndpointType' service model/model_meta.go model/model_extra.go controller/model.go 2>/dev/null || true
 rg -n '^(type .*struct|func ).*(Model|model)' model/model_meta.go model/model_extra.go model/vendor_meta.go model/vendor_management.go 2>/dev/null || true
 printf '%s\n' '## Frontend channel symbols'; require_file web/src/features/channels/constants.ts
 rg -n 'CHANNEL_TYPE_OPTIONS|export const [A-Z][A-Z0-9_]*_TYPES' web/src/features/channels/constants.ts
}
relay_map(){
 local provider="${1:-}"; [[ "$provider" =~ ^[a-z0-9_-]+$ ]] || { usage >&2; return 2; }
 local dir="relay/channel/$provider"; [[ -d "$dir" ]] || { printf 'provider directory not found: %s\n' "$dir" >&2; return 1; }
 printf '## Files (%s)\n' "$dir"; find "$dir" -type f -name '*.go' -print | sort
 while IFS= read -r f; do printf '\n### %s\n' "$f"; rg -nH '^(type[[:space:]]+[A-Za-z0-9_]+[[:space:]]+struct|func[[:space:]])' "$f" || true; done < <(find "$dir" -type f -name '*.go' -print | sort)
 for f in relay/relay_adaptor.go; do printf '\n### %s\n' "$f"; rg -nH '^(type[[:space:]]+[A-Za-z0-9_]+[[:space:]]+struct|func[[:space:]])' "$f" || true; done
 while IFS= read -r f; do printf '\n### %s\n' "$f"; rg -nH '^(type[[:space:]]+[A-Za-z0-9_]+[[:space:]]+struct|func[[:space:]])' "$f" || true; done < <(find relaykit/relayconvert -type f -name '*.go' -print | sort)
}
controller_map(){
 for f in controller/channel*.go controller/relay.go; do [[ -f "$f" ]] || continue; printf '\n## %s\n' "$f"; print_go_ranges "$f"; done
 while IFS= read -r f; do printf '\n## %s named UI blocks\n' "$f"; print_ts_named_ranges "$f"; done < <(find web/src/features/channels/components/drawers -maxdepth 1 -type f -name 'channel-mutate-*.tsx' -print | sort)
 while IFS= read -r f; do printf '\n## %s named UI blocks\n' "$f"; print_ts_named_ranges "$f"; done < <(find web/src/features/channels/components/dialogs web/src/features/xpiki-quota -maxdepth 1 -type f \( -name 'param-override-*.tsx' -o -name 'xpiki-quota-*.tsx' -o -name 'index.tsx' \) -print | sort)
}
pricing(){
 printf '%s\n' '## Backend pricing/quota/billing/model symbols'
 while IFS= read -r f; do printf '\n### %s\n' "$f"; rg -nH '^(type .*struct|func |const |var )' "$f" || true; done < <(find setting service relay model controller -type f -name '*.go' -print | rg '(^|/)(.*(ratio|billing|quota|pricing|price|tiered|cost|usage|model_pric|selling|comparison).*)\.go$' | sort)
 printf '%s\n' '## Frontend pricing/model/quota exports'
 while IFS= read -r f; do rg -nH '^export (const|function|type|interface|schema)' "$f" || true; done < <(find web/src/features/pricing web/src/features/model-pricing web/src/features/models web/src/features/xpiki-quota -type f \( -name '*.ts' -o -name '*.tsx' \) -print | sort)
}
domain(){
 local name="${1:-}"; [[ "$name" =~ ^(auth-routing|models|users-groups|logs-settings)$ ]] || { usage >&2; return 2; }; local -a files=()
 case "$name" in
  auth-routing) mapfile -t files < <(find router middleware -type f -name '*.go' -print; printf '%s\n' model/token.go model/ability.go controller/token.go);;
  models) mapfile -t files < <(find model controller service -type f -name '*.go' | rg '(^|/)(model|vendor|catalog|sync|missing_models|channel)([^/]*)?\.go$' || true);;
  users-groups) mapfile -t files < <(find model controller service -type f -name '*.go' | rg '(^|/)(user|group|redemption|topup|billing|oauth|verification|auth|login|passkey|twofa)([^/]*)?\.go$' || true);;
  logs-settings) mapfile -t files < <(find model controller service setting -type f -name '*.go' | rg '(^|/)(log|usedata|performance|option|system|setting|billing|usage|stat|dashboard)([^/]*)?\.go$' || true);;
 esac
 for f in "${files[@]}"; do [[ -f "$f" ]] || continue; printf '\n### %s\n' "$f"; rg -nH '^type .*struct' "$f" || true; print_go_ranges "$f"; done
}
web_map(){
 local feature="${1:-}"; if [[ -z "$feature" ]]; then
  printf '%s\n' '## Feature directories'; find web/src/features -mindepth 1 -maxdepth 1 -type d -print | sort
  printf '%s\n' '## Shared directories'; for d in web/src/routes web/src/components web/src/i18n/locales; do printf '%s\n' "$d"; find "$d" -mindepth 1 -type d -print 2>/dev/null | sort; done; return; fi
 [[ "$feature" =~ ^[A-Za-z0-9_-]+$ ]] || return 2; local dir="web/src/features/$feature"; [[ -d "$dir" ]] || { printf 'feature directory not found: %s\n' "$dir" >&2; return 1; }
 find "$dir" -type f -print | sort; while IFS= read -r f; do rg -nH '^export (const|function|type|interface|schema)' "$f" || true; done < <(find "$dir" -type f \( -name '*.ts' -o -name '*.tsx' \) -print | sort)
}
wip(){
 printf '%s\n' '## Working tree'; git status --short
 printf '%s\n' '## Diff stat'; git diff --stat
 printf '%s\n' '## Hunk headers'; git diff -U0 | rg '^(diff --git|@@ )' || true
}
i18n(){
 local keyword="${1:-}"; [[ -n "$keyword" ]] || { usage >&2; return 2; }
 require_file web/src/i18n/locales/vi.json
 rg -nHi "$keyword" web/src/i18n/locales/vi.json | head -80
}
case "${1:-}" in
 overview) [[ $# -eq 1 ]] && overview || { usage >&2; exit 2; };;
 channel-types) [[ $# -eq 1 ]] && channel_types || { usage >&2; exit 2; };;
 relay) [[ $# -eq 2 ]] && relay_map "$2" || { usage >&2; exit 2; };;
 controller-map) [[ $# -eq 1 ]] && controller_map || { usage >&2; exit 2; };;
 pricing) [[ $# -eq 1 ]] && pricing || { usage >&2; exit 2; };;
 domain) [[ $# -eq 2 ]] && domain "$2" || { usage >&2; exit 2; };;
 web) [[ $# -le 2 ]] && web_map "${2:-}" || { usage >&2; exit 2; };;
 wip) [[ $# -eq 1 ]] && wip || { usage >&2; exit 2; };;
 i18n) [[ $# -eq 2 ]] && i18n "$2" || { usage >&2; exit 2; };;
 *) usage >&2; exit 2;;
esac
