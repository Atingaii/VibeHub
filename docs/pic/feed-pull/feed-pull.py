#!/usr/bin/env python3
"""VibeShop Feed 读扩散流程图（Pull 模式，大 V ≥ 2000 粉丝）— 极简柔和风格"""

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
g = graphviz.Digraph('feed_pull', format='png')
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

# ===== 粉丝 =====
g.node('fan', '粉丝\n(刷新 Feed)', shape='ellipse',
       fillcolor=BLUE, color=BLUE_BD, width='1.4', height='0.6')

# ===== 请求 =====
g.node('refresh', 'GET /api/feed', fillcolor=BLUE, color=BLUE_BD)

# ===== ① 实时聚合 =====
with g.subgraph(name='cluster_aggregate') as c:
    c.attr(
        label='  ① 实时聚合', style='dashed,rounded',
        color=BORDER, bgcolor=GROUP_BG,
        fontsize='12', fontcolor=LABEL_CLR,
        penwidth='1.2', labeljust='l', margin='16',
    )
    c.node('inbox',     '读取 Inbox\nRedis SortedSet\n(推模式内容)',
           fillcolor=CORAL, color=CORAL_BD)
    c.node('pull_bigv', '拉取关注大V最新\nPostgreSQL 查询',
           fillcolor=PURPLE, color=PURPLE_BD)
    c.node('merge',     '合并\nInbox(推) + 大V(拉)',
           fillcolor=AMBER, color=AMBER_BD)

# ===== ② 排序 & 分页 =====
with g.subgraph(name='cluster_rank') as c:
    c.attr(
        label='  ② 排序 & 分页', style='dashed,rounded',
        color=BORDER, bgcolor=GROUP_BG,
        fontsize='12', fontcolor=LABEL_CLR,
        penwidth='1.2', labeljust='l', margin='16',
    )
    c.node('wilson', 'Wilson Score\n+ 时间衰减',   fillcolor=GREEN, color=GREEN_BD)
    c.node('cursor', 'Cursor 分页\n返回 Top N',    fillcolor=GREEN, color=GREEN_BD)

# ===== 响应 =====
g.node('response', 'Feed 响应\nJSON', fillcolor=GREEN, color=GREEN_BD)

# ===== 大V数据源 =====
with g.subgraph(name='cluster_bigv') as c:
    c.attr(
        label='  大 V 数据 (≥ 2000 粉丝)', style='dashed,rounded',
        color=BORDER, bgcolor=GROUP_BG,
        fontsize='11', fontcolor=LABEL_CLR,
        penwidth='1.0', margin='14',
    )
    c.node('bigv1', '大V-A 最新博文', fillcolor=GRAY_FILL, color=GRAY_BD,
           shape='note', width='1.4', fontsize='10')
    c.node('bigv2', '大V-B 最新博文', fillcolor=GRAY_FILL, color=GRAY_BD,
           shape='note', width='1.4', fontsize='10')
    c.node('bigv3', '大V-C 最新博文', fillcolor=GRAY_FILL, color=GRAY_BD,
           shape='note', width='1.4', fontsize='10')

# ===== 连接 =====
g.edge('fan', 'refresh')
g.edge('refresh', 'inbox')
g.edge('refresh', 'pull_bigv')

g.edge('bigv1', 'pull_bigv', style='dashed', penwidth='0.7')
g.edge('bigv2', 'pull_bigv', style='dashed', penwidth='0.7')
g.edge('bigv3', 'pull_bigv', style='dashed', penwidth='0.7')

g.edge('inbox', 'merge')
g.edge('pull_bigv', 'merge')
g.edge('merge', 'wilson')
g.edge('wilson', 'cursor')
g.edge('cursor', 'response')

# ── 渲染 ─────────────────────────────────────────────
g.render(filename=os.path.join(output_dir, 'feed-pull'), cleanup=True)
print(f"✅ Feed读扩散图已生成: {output_dir}/feed-pull.png")
