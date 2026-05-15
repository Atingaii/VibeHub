#!/usr/bin/env python3
"""VibeShop AI 总结链路数据流图 — 极简柔和风格"""

import graphviz
import os

output_dir = os.path.dirname(os.path.abspath(__file__))

# ── 设计令牌 ─────────────────────────────────────────
BG        = '#FAFBFC'
GROUP_BG  = '#FFFFFF'
BORDER    = '#D0D5DD'
LABEL_CLR = '#344054'
TEXT_CLR  = '#1D2939'

BLUE      = '#D6E4FF';  BLUE_BD   = '#84ADFF'
GREEN     = '#D3F8DF';  GREEN_BD  = '#6CE9A6'
PURPLE    = '#E8DAFF';  PURPLE_BD = '#B692F6'
AMBER     = '#FEF0C7';  AMBER_BD  = '#FDB022'
CORAL     = '#FFE4E8';  CORAL_BD  = '#FD6F8E'
GRAY_FILL = '#F2F4F7';  GRAY_BD   = '#98A2B3'

FONT      = 'Noto Sans CJK SC'
EDGE_CLR  = '#98A2B3'

# ── 画布 ─────────────────────────────────────────────
g = graphviz.Digraph('ai_summary', format='png')
g.attr(
    rankdir='LR', dpi='200', bgcolor=BG, pad='0.8',
    fontname=FONT, nodesep='0.6', ranksep='0.9',
    margin='0.3',
)
g.attr('node',
    fontname=FONT, fontsize='11', fontcolor=TEXT_CLR,
    style='filled,rounded', shape='box',
    penwidth='1.0', height='0.6', width='2.0',
)
g.attr('edge',
    fontname=FONT, fontsize='9', fontcolor='#667085',
    color=EDGE_CLR, arrowsize='0.7', penwidth='0.9',
)

# ===== ① 触发 =====
with g.subgraph(name='cluster_trigger') as c:
    c.attr(
        label='  ① 触发', style='dashed,rounded',
        color=BORDER, bgcolor=GROUP_BG,
        fontsize='12', fontcolor=LABEL_CLR,
        penwidth='1.2', labeljust='l', margin='16',
    )
    c.node('publish',    '博文发布\n写入 PostgreSQL', fillcolor=PURPLE, color=PURPLE_BD)
    c.node('nats_event', 'NATS 发布\ncontent.published',  fillcolor=PURPLE, color=PURPLE_BD)

# ===== ② AI Consumer =====
with g.subgraph(name='cluster_ai') as c:
    c.attr(
        label='  ② AI Consumer', style='dashed,rounded',
        color=BORDER, bgcolor=GROUP_BG,
        fontsize='12', fontcolor=LABEL_CLR,
        penwidth='1.2', labeljust='l', margin='16',
    )
    c.node('ai_consumer', 'AI Consumer\n消费消息',   fillcolor=AMBER, color=AMBER_BD)
    c.node('extract',     '提取内容\n准备 Prompt',   fillcolor=AMBER, color=AMBER_BD)

# ===== ③ MCP Gateway =====
with g.subgraph(name='cluster_mcp') as c:
    c.attr(
        label='  ③ MCP Gateway  统一调度', style='dashed,rounded',
        color=BORDER, bgcolor=GROUP_BG,
        fontsize='12', fontcolor=LABEL_CLR,
        penwidth='1.2', labeljust='l', margin='16',
    )
    c.node('mcp_gw', 'MCP Gateway\n路由 · 限流 · Fallback',
           fillcolor=CORAL, color=CORAL_BD)
    c.node('ollama', 'Ollama (本地)',  fillcolor=BLUE, color=BLUE_BD,
           shape='component', fontsize='10', width='1.4')
    c.node('openai', 'OpenAI GPT-4',  fillcolor=BLUE, color=BLUE_BD,
           shape='component', fontsize='10', width='1.4')
    c.node('claude', 'Claude',         fillcolor=BLUE, color=BLUE_BD,
           shape='component', fontsize='10', width='1.4')

# ===== ④ 结果存储 =====
with g.subgraph(name='cluster_store') as c:
    c.attr(
        label='  ④ 结果存储', style='dashed,rounded',
        color=BORDER, bgcolor=GROUP_BG,
        fontsize='12', fontcolor=LABEL_CLR,
        penwidth='1.2', labeljust='l', margin='16',
    )
    c.node('write_pg',    '写入 PostgreSQL', fillcolor=GREEN, color=GREEN_BD)
    c.node('cache_redis', '缓存到 Redis',   fillcolor=GREEN, color=GREEN_BD)

# ===== 连接 =====
g.edge('publish', 'nats_event', label='异步')
g.edge('nats_event', 'ai_consumer')
g.edge('ai_consumer', 'extract')
g.edge('extract', 'mcp_gw', label='请求总结')

g.edge('mcp_gw', 'ollama', style='dashed', penwidth='0.7')
g.edge('mcp_gw', 'openai', style='dashed', penwidth='0.7')
g.edge('mcp_gw', 'claude', style='dashed', penwidth='0.7')

# 模型返回 → 存储（只从 gateway 连一根线到存储，更简洁）
g.edge('mcp_gw', 'write_pg', label='返回结果', style='dashed', penwidth='0.7')
g.edge('write_pg', 'cache_redis', label='同步缓存')

# ── 渲染 ─────────────────────────────────────────────
g.render(filename=os.path.join(output_dir, 'ai-summary'), cleanup=True)
print(f"✅ AI总结链路图已生成: {output_dir}/ai-summary.png")
