#!/usr/bin/env bash
# scripts/check-docs.sh — 文档-代码一致性检查（doc-lint）
# 用法: ./scripts/check-docs.sh 或 make doc-lint
# 退出码: 0=全通过, 1=有问题

set -euo pipefail

RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[0;33m'
NC='\033[0m' # No Color

ERRORS=0
WARNINGS=0

log_error() {
  echo -e "${RED}✗ ERROR: $1${NC}"
  ERRORS=$((ERRORS + 1))
}

log_warn() {
  echo -e "${YELLOW}⚠ WARN: $1${NC}"
  WARNINGS=$((WARNINGS + 1))
}

log_ok() {
  echo -e "${GREEN}✓ $1${NC}"
}

echo "========================================="
echo "  VibeShop doc-lint (scripts/check-docs.sh)"
echo "========================================="
echo ""

# -----------------------------------------------------------
# 1. 检查环境变量两端对账
#    docs/dev-workflow.md 中列出的变量 vs configs/dev.yaml 中的实际使用
# -----------------------------------------------------------
echo "--- [1/4] 环境变量对账 ---"

if [[ -f docs/dev-workflow.md ]] && [[ -f configs/dev.yaml ]]; then
  # 从 dev-workflow.md 提取反引号包裹的环境变量名
  DOC_VARS=$(grep -oP '`[A-Z][A-Z_]+`' docs/dev-workflow.md | tr -d '`' | sort -u)
  
  # 检查每个文档中声明的变量是否在 configs/ 或代码中能找到对应
  MISSING_COUNT=0
  for var in $DOC_VARS; do
    # VIBESHOP_* 是 Docker Compose 专用变量（docs/dev-workflow.md 中
    # "Docker 专用环境变量" 节明确说明应用不直接读取）。校验拆两步：
    #   (1) 在 deploy/docker/.env.example 中必须出现（活跃赋值或注释模板均算声明）
    #   (2) 在 deploy/docker/docker-compose*.yml 中必须有 *非注释* 的真实引用
    #       端口类变量 VIBESHOP_*_HOST_PORT 默认走 ${VAR:-默认}，
    #       注释保留在 .env.example 中表示用户可选覆盖。
    if [[ "$var" == VIBESHOP_* ]]; then
      if ! grep -Eq "^[[:space:]]*#?[[:space:]]*${var}=" deploy/docker/.env.example 2>/dev/null; then
        log_warn "Docker 变量 $var 在 deploy/docker/.env.example 中未声明（活跃或注释模板均可）"
        MISSING_COUNT=$((MISSING_COUNT + 1))
      fi
      # 同时覆盖 ${VAR} 与 ${VAR:-default}，并排除以 # 开头的 yaml 注释行
      # 锚点 [^[:space:]#] 要求"行首空白后第一个非空白字符不是 #"，
      # 否则 [[:space:]]* 可能匹配 0 长度而 [^#] 落到空白上，导致注释行漏判
      if ! grep -Elq "^[[:space:]]*[^[:space:]#].*\\\$\\{${var}(:-[^}]*)?\\}" deploy/docker/docker-compose*.yml 2>/dev/null; then
        log_warn "Docker 变量 $var 未在 deploy/docker/docker-compose*.yml 中被非注释行引用"
        MISSING_COUNT=$((MISSING_COUNT + 1))
      fi
      continue
    fi
    # 跳过不需要在 yaml 中出现的变量（直接是环境变量覆盖）
    # 在 configs 或者 internal/ 中应有对应（小写形式或原始形式）
    VAR_LOWER=$(echo "$var" | tr '[:upper:]' '[:lower:]')
    if ! grep -rq "$var\|$VAR_LOWER" configs/ 2>/dev/null; then
      # 也检查 internal/ 代码（如果存在）
      if [[ -d internal/ ]]; then
        if ! grep -rq "$var" internal/ 2>/dev/null; then
          log_warn "环境变量 $var（docs/dev-workflow.md 中声明）在 configs/ 和 internal/ 中未找到引用"
          MISSING_COUNT=$((MISSING_COUNT + 1))
        fi
      fi
    fi
  done
  
  if [[ $MISSING_COUNT -eq 0 ]]; then
    log_ok "环境变量对账通过（文档变量在配置/代码中可找到）"
  fi
else
  log_warn "docs/dev-workflow.md 或 configs/dev.yaml 不存在，跳过环境变量对账"
fi

echo ""

# -----------------------------------------------------------
# 2. 检查代码锚点存在性（R3）
#    扫描所有 .md 文件中的 path:function 格式锚点
# -----------------------------------------------------------
echo "--- [2/4] 代码锚点存在性检查 ---"

# 查找 md 文件中类似 `internal/xxx/yyy.go:FuncName` 的锚点
ANCHORS=$(grep -rhoP '`[a-zA-Z_/]+\.go:[a-zA-Z_]+`' docs/ AGENTS.md README.md 2>/dev/null | tr -d '`' | sort -u || true)

ANCHOR_MISSING=0
if [[ -n "$ANCHORS" ]]; then
  for anchor in $ANCHORS; do
    FILE_PATH=$(echo "$anchor" | cut -d: -f1)
    FUNC_NAME=$(echo "$anchor" | cut -d: -f2)
    
    if [[ -f "$FILE_PATH" ]]; then
      # 文件存在，检查函数/变量是否存在
      if ! grep -q "$FUNC_NAME" "$FILE_PATH" 2>/dev/null; then
        log_warn "锚点 $anchor: 文件存在但未找到符号 '$FUNC_NAME'"
        ANCHOR_MISSING=$((ANCHOR_MISSING + 1))
      fi
    else
      # 文件不存在——如果是规划中的路径（项目还未实现），降为 info 不报错
      if [[ "$FILE_PATH" == internal/* ]] || [[ "$FILE_PATH" == cmd/* ]] || [[ "$FILE_PATH" == pkg/* ]] || [[ "$FILE_PATH" == scripts/* ]]; then
        : # 规划路径，项目尚未实现，静默跳过
      else
        log_warn "锚点 $anchor: 文件 '$FILE_PATH' 不存在"
        ANCHOR_MISSING=$((ANCHOR_MISSING + 1))
      fi
    fi
  done
  
  if [[ $ANCHOR_MISSING -eq 0 ]]; then
    log_ok "代码锚点检查通过（已有文件中的符号均存在或为规划路径）"
  fi
else
  log_ok "未发现代码锚点引用（项目尚处于设计阶段）"
fi

echo ""

# -----------------------------------------------------------
# 3. Doc-Impact 格式检查（R6）
#    检查最近 N 个 commit 的 Doc-Impact 标签
# -----------------------------------------------------------
echo "--- [3/4] Doc-Impact 格式检查 ---"

if git rev-parse --is-inside-work-tree >/dev/null 2>&1; then
  # 检查最近 10 个 commit
  COMMIT_COUNT=$(git log --oneline -10 2>/dev/null | wc -l)
  DOC_IMPACT_MISSING=0
  
  for i in $(seq 1 "$COMMIT_COUNT"); do
    COMMIT_HASH=$(git log --format='%H' -n 1 --skip=$((i-1)) 2>/dev/null)
    COMMIT_MSG=$(git log --format='%B' -n 1 "$COMMIT_HASH" 2>/dev/null)
    COMMIT_SHORT=$(git log --format='%h %s' -n 1 "$COMMIT_HASH" 2>/dev/null)
    
    # 检查是否有 Doc-Impact 或 Doc impact 行
    if echo "$COMMIT_MSG" | grep -qiP '^Doc[- ]?[Ii]mpact:'; then
      # 检查格式是否正确：Doc-Impact: none 或 Doc-Impact: 文件列表
      DOC_LINE=$(echo "$COMMIT_MSG" | grep -iP '^Doc[- ]?[Ii]mpact:' | head -1)
      
      # 检查是否用了标准格式 Doc-Impact（Title-Case + 连字符）
      if ! echo "$DOC_LINE" | grep -qP '^Doc-Impact:'; then
        log_warn "commit $COMMIT_SHORT: Doc-Impact 格式不标准（应为 'Doc-Impact:'，Title-Case+连字符）"
      fi
    else
      log_warn "commit $COMMIT_SHORT: 缺少 Doc-Impact 标签"
      DOC_IMPACT_MISSING=$((DOC_IMPACT_MISSING + 1))
    fi
  done
  
  if [[ $DOC_IMPACT_MISSING -eq 0 ]]; then
    log_ok "最近 $COMMIT_COUNT 个 commit 的 Doc-Impact 标签格式正确"
  fi
else
  log_warn "不在 git 仓库中，跳过 Doc-Impact 检查"
fi

echo ""

# -----------------------------------------------------------
# 4. 文档完整性检查
#    确认关键文档文件存在
# -----------------------------------------------------------
echo "--- [4/4] 文档完整性检查 ---"

REQUIRED_DOCS=(
  "AGENTS.md"
  "README.md"
  "docs/dev-workflow.md"
  "docs/change-impact.md"
  "docs/plan.md"
  "docs/architecture.md"
  "docs/code-map.md"
)

for doc in "${REQUIRED_DOCS[@]}"; do
  if [[ -f "$doc" ]]; then
    log_ok "$doc 存在"
  else
    log_error "$doc 缺失！"
  fi
done

echo ""

# -----------------------------------------------------------
# 汇总
# -----------------------------------------------------------
echo "========================================="
echo "  汇总: ${ERRORS} 错误, ${WARNINGS} 警告"
echo "========================================="

if [[ $ERRORS -gt 0 ]]; then
  echo -e "${RED}doc-lint 失败！请修复上述错误后再提交。${NC}"
  exit 1
elif [[ $WARNINGS -gt 0 ]]; then
  echo -e "${YELLOW}doc-lint 通过（有警告，建议修复）。${NC}"
  exit 0
else
  echo -e "${GREEN}doc-lint 全部通过！✓${NC}"
  exit 0
fi
