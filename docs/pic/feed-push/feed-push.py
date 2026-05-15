#!/usr/bin/env python3
"""VibeShop Feed 写扩散流程图（Push 模式，粉丝 < 2000）— 极简柔和风格"""

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
g = graphviz.Digraph('feed_push', format='png')
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

# ===== 作者 =====
g.node('author', '普通作者\n(< 2000 粉丝)', shape='ellipse',
       fillcolor=BLUE, color=BLUE_BD, width='1.4', height='0.6')

# ===== ① 发布 =====
with g.subgraph(name='cluster_publish') as c:
    c.attr(
        label='  ① 发布', style='dashed,rounded',
        color=BORDER, bgcolor=GROUP_BG,
        fontsize='12', fontcolor=LABEL_CLR,
        penwidth='1.2', labeljust='l', margin='16',
    )
    c.node('publish',    '发布博文\n写入 PostgreSQL', fillcolor=PURPLE, color=PURPLE_BD)
    c.node('nats_event', 'NATS 发布\ncontent.published',  fillcolor=PURPLE, color=PURPLE_BD)

# ===== ② Consumer =====
with g.subgraph(name='cluster_consume') as c:
    c.attr(
        label='  ② Feed Consumer', style='dashed,rounded',
        color=BORDER, bgcolor=GROUP_BG,
        fontsize='12', fontcolor=LABEL_CLR,
        penwidth='1.2', labeljust='l', margin='16',
    )
    c.node('consumer', 'Feed Consumer\n消费消息', fillcolor=AMBER, color=AMBER_BD)
    c.node('fan_list', '读取粉丝列表',            fillcolor=AMBER, color=AMBER_BD)

# ===== ③ 写扩散 =====
with g.subgraph(name='cluster_fanout') as c:
    c.attr(
        label='  ③ 写扩散 Push', style='dashed,rounded',
        color=BORDER, bgcolor=GROUP_BG,
        fontsize='12', fontcolor=LABEL_CLR,
        penwidth='1.2', labeljust='l', margin='16',
    )
    c.node('zadd', 'ZADD 写入\nRedis SortedSet',
           fillcolor=CORAL, color=CORAL_BD)
    c.node('fan1', '粉丝 A Inbox', fillcolor=GRAY_FILL, color=GRAY_BD,
           shape='note', width='1.4')
    c.node('fan2', '粉丝 B Inbox', fillcolor=GRAY_FILL, color=GRAY_BD,
           shape='note', width='1.4')
    c.node('fan3', '粉丝 C Inbox', fillcolor=GRAY_FILL, color=GRAY_BD,
           shape='note', width='1.4')

# ===== ④ 读取 =====
g.node('fan_read', '粉丝刷新 Feed\nZRANGEBYSCORE\ncursor 分页',
       fillcolor=GREEN, color=GREEN_BD)
g.node('fan', '粉丝', shape='ellipse',
       fillcolor=BLUE, color=BLUE_BD, width='1.0', height='0.5')

# ===== 连接 =====
g.edge('author', 'publish')
g.edge('publish', 'nats_event', label='异步')
g.edge('nats_event', 'consumer')
g.edge('consumer', 'fan_list')
g.edge('fan_list', 'zadd', label='遍历粉丝')

g.edge('zadd', 'fan1', penwidth='0.7')
g.edge('zadd', 'fan2', penwidth='0.7')
g.edge('zadd', 'fan3', penwidth='0.7')

g.edge('fan1', 'fan_read', style='dashed', penwidth='0.7')
g.edge('fan2', 'fan_read', style='dashed', penwidth='0.7')
g.edge('fan3', 'fan_read', style='dashed', penwidth='0.7')
g.edge('fan_read', 'fan')

# ── 渲染 ─────────────────────────────────────────────
g.render(filename=os.path.join(output_dir, 'feed-push'), cleanup=True)
print(f"✅ Feed写扩散图已生成: {output_dir}/feed-push.png")
