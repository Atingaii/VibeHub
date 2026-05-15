#!/usr/bin/env python3
"""VibeShop 整体架构图 — 极简柔和风格 v6
核心策略（彻底解决线条拖沓、跨域过远问题）：
  1. TB 布局，保持传统三层架构的直觉
  2. 基础设施节点按 MySQL | Redis | PG | NATS 排列
     MySQL 对齐交易域左侧，Redis 对齐交易域右侧/内容域
     PG 对齐内容域，NATS 对齐 AI 域
  3. 只画向下的连接（业务模块 → 基础设施），不画反向回头线
  4. 消息线路（→ NATS）用蓝色区分，数据存储线用灰色
  5. 精简连线数量，只保留最核心的数据流关系
  6. 使用 splines='spline' 保证曲线平滑
"""

import graphviz
import os

output_dir = os.path.dirname(os.path.abspath(__file__))

# ── 统一设计令牌 ──────────────────────────────────────
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

# 消息通道使用蓝色
MSG_CLR   = '#528BFF'

FONT      = 'Noto Sans CJK SC'
EDGE_CLR  = '#98A2B3'

# ── 创建画布 ──────────────────────────────────────────
g = graphviz.Digraph('architecture', format='png')
g.attr(
    rankdir='TB', dpi='200', bgcolor=BG, pad='0.4',
    fontname=FONT, nodesep='0.5', ranksep='0.65',
    margin='0.2', splines='spline', compound='true',
    newrank='true',
)
g.attr('node',
    fontname=FONT, fontsize='10', fontcolor=TEXT_CLR,
    style='filled,rounded', shape='box',
    penwidth='1.0', height='0.4', width='1.0',
)
g.attr('edge',
    fontname=FONT, fontsize='8', fontcolor='#667085',
    color=EDGE_CLR, arrowsize='0.5', penwidth='0.7',
)

# ===== ① 客户端层 =====
with g.subgraph(name='cluster_client') as c:
    c.attr(
        label='  客户端层', style='dashed,rounded',
        color=BORDER, bgcolor=GROUP_BG,
        fontsize='11', fontcolor=LABEL_CLR,
        penwidth='1.0', labeljust='l', margin='16',
    )
    c.node('web',        'Web\n(Next.js)',      fillcolor=BLUE, color=BLUE_BD, width='1.1')
    c.node('mobile',     'Mobile\nApp',         fillcolor=BLUE, color=BLUE_BD, width='1.1')
    c.node('mcp_client', 'MCP\nClient',         fillcolor=BLUE, color=BLUE_BD, width='1.1')

# ===== ② Gin HTTP Server 及三个业务域 =====
with g.subgraph(name='cluster_server') as s:
    s.attr(
        label='  Gin HTTP Server  :8080', style='dashed,rounded',
        color=BORDER, bgcolor=GROUP_BG,
        fontsize='11', fontcolor=LABEL_CLR,
        penwidth='1.0', labeljust='l', margin='16',
    )
    s.node('middleware', 'Middleware:  JWT · RateLimit · Logger · Trace',
           fillcolor=AMBER, color=AMBER_BD, width='7.5', fontsize='10', height='0.4')

    # ── 交易域（左）──
    with s.subgraph(name='cluster_trade') as t:
        t.attr(
            label='  交易域', style='dashed,rounded',
            color='#C6EFCE', bgcolor='#F6FEF9',
            fontsize='9', fontcolor='#667085',
            penwidth='0.8', margin='12',
        )
        for nid, nlabel in [('user', '用户'), ('product', '商品'), ('order', '订单'),
                             ('groupbuy', '拼团'), ('coupon', '优惠券'), ('lottery', '抽奖')]:
            t.node(nid, nlabel, fillcolor=GREEN, color=GREEN_BD, width='0.8', height='0.35')

    # ── 内容域（中）──
    with s.subgraph(name='cluster_content') as ct:
        ct.attr(
            label='  内容域', style='dashed,rounded',
            color='#D6BBFB', bgcolor='#FAF5FF',
            fontsize='9', fontcolor='#667085',
            penwidth='0.8', margin='12',
        )
        ct.node('content', '内容', fillcolor=PURPLE, color=PURPLE_BD, width='1.0', height='0.35')
        ct.node('feed',    'Feed',  fillcolor=PURPLE, color=PURPLE_BD, width='1.0', height='0.35')

    # ── AI 域（右）──
    with s.subgraph(name='cluster_ai') as a:
        a.attr(
            label='  AI 域', style='dashed,rounded',
            color='#FECDD6', bgcolor='#FFF5F6',
            fontsize='9', fontcolor='#667085',
            penwidth='0.8', margin='12',
        )
        a.node('ai',     'AI 总结',        fillcolor=CORAL, color=CORAL_BD, width='1.0', height='0.35')
        a.node('mcp_gw', 'MCP Gateway',    fillcolor=CORAL, color=CORAL_BD, width='1.0', height='0.35')

# ===== ③ 数据与基础设施层 =====
# 关键：节点顺序经过精心安排，与上方域位置严格对齐
# MySQL 对齐交易域左侧, Redis 对齐交易域右侧, PG 对齐内容域, NATS 对齐 AI 域
with g.subgraph(name='cluster_infra') as i:
    i.attr(
        label='  数据与基础设施层', style='dashed,rounded',
        color=BORDER, bgcolor=GROUP_BG,
        fontsize='11', fontcolor=LABEL_CLR,
        penwidth='1.0', labeljust='l', margin='16',
    )
    i.node('mysql', 'MySQL 8.0+\n交易数据',
           fillcolor=GRAY_FILL, color=GRAY_BD, shape='cylinder', width='1.4')
    i.node('redis', 'Redis 7.x\n缓存/Feed/库存',
           fillcolor=GRAY_FILL, color=GRAY_BD, shape='cylinder', width='1.4')
    i.node('pg',    'PostgreSQL 16\n内容数据',
           fillcolor=GRAY_FILL, color=GRAY_BD, shape='cylinder', width='1.4')
    i.node('nats',  'NATS JetStream\n消息队列',
           fillcolor=GRAY_FILL, color=GRAY_BD, shape='cylinder', width='1.4')

# ===== 连接线 =====
# 设计原则：只画向下的短连接，每条线连接的节点在水平方向上尽量对齐

# (A) 客户端 → Middleware（一条代表线）
g.edge('mobile', 'middleware', label='HTTP/SSE', style='dashed')

# (B) 交易域 → MySQL（短直线，垂直对齐）
g.edge('order', 'mysql', label='GORM', style='dashed')

# (C) 交易域 → Redis（短直线，几乎垂直对齐）
g.edge('groupbuy', 'redis', label='Lua 预扣', style='dashed')

# (D) 内容域 → Redis（Feed 用 Redis SortedSet）
g.edge('feed', 'redis', label='SortedSet', style='dashed', constraint='false')

# (E) 内容域 → PG（短直线，垂直对齐）
g.edge('content', 'pg', label='GORM', style='dashed')

# (F) NATS 消息发布线 — 使用蓝色区分
# order 发布订单事件到 NATS
g.edge('order', 'nats', label='订单事件', style='dashed',
       color=MSG_CLR, fontcolor=MSG_CLR)
# feed 从 NATS 消费写扩散消息
g.edge('feed', 'nats', label='写扩散', style='dashed',
       color=MSG_CLR, fontcolor=MSG_CLR, constraint='false')

# (G) AI 域内部 + AI→NATS
g.edge('ai', 'mcp_gw', label='统一调度')
g.edge('ai', 'nats', label='AI 触发', style='dashed',
       color=MSG_CLR, fontcolor=MSG_CLR)

# ===== 布局精确控制 =====

# 客户端同行
with g.subgraph() as s:
    s.attr(rank='same')
    s.node('web'); s.node('mobile'); s.node('mcp_client')

# 基础设施同行 & 顺序控制
with g.subgraph() as s:
    s.attr(rank='same')
    s.node('mysql'); s.node('redis'); s.node('pg'); s.node('nats')

# 三个域水平排列：交易域(左) → 内容域(中) → AI域(右)
g.edge('coupon', 'content', style='invis')
g.edge('feed', 'ai', style='invis')

# 基础设施节点顺序锁定：MySQL → Redis → PG → NATS
g.edge('mysql', 'redis', style='invis')
g.edge('redis', 'pg', style='invis')
g.edge('pg', 'nats', style='invis')

# ── 渲染 ─────────────────────────────────────────────
g.render(filename=os.path.join(output_dir, 'architecture'), cleanup=True)
print(f"✅ 整体架构图已生成: {output_dir}/architecture.png")
