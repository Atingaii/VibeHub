#!/usr/bin/env python3
"""VibeShop 购物链路（拼团）数据流图 — 极简柔和风格"""

import graphviz
import os

output_dir = os.path.dirname(os.path.abspath(__file__))

# ── 设计令牌（与 architecture.py 统一色板）────────────
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
g = graphviz.Digraph('groupbuy_flow', format='png')
g.attr(
    rankdir='TB', dpi='200', bgcolor=BG, pad='0.8',
    fontname=FONT, nodesep='0.6', ranksep='0.75',
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

# ===== 用户入口 =====
g.node('user', '用户', shape='ellipse',
       fillcolor=BLUE, color=BLUE_BD, width='1.0', height='0.5')

# ===== ① 开团阶段 =====
with g.subgraph(name='cluster_initiate') as c:
    c.attr(
        label='  ① 开团阶段', style='dashed,rounded',
        color=BORDER, bgcolor=GROUP_BG,
        fontsize='12', fontcolor=LABEL_CLR,
        penwidth='1.2', labeljust='l', margin='16',
    )
    c.node('open_group',      '选择商品 + 拼团活动',     fillcolor=BLUE,  color=BLUE_BD)
    c.node('handler_validate', 'Handler 验证\n活动 / 库存 / 资格', fillcolor=GRAY_FILL, color=GRAY_BD)
    c.node('redis_deduct',     'Redis Lua 原子预扣库存',  fillcolor=CORAL, color=CORAL_BD)

# ===== ② 订单创建 =====
with g.subgraph(name='cluster_order') as c:
    c.attr(
        label='  ② 订单创建', style='dashed,rounded',
        color=BORDER, bgcolor=GROUP_BG,
        fontsize='12', fontcolor=LABEL_CLR,
        penwidth='1.2', labeljust='l', margin='16',
    )
    c.node('create_order', '创建拼团订单 (MySQL)', fillcolor=GREEN, color=GREEN_BD)
    c.node('order_status', '订单状态：待支付 / 等待参团', fillcolor=GREEN, color=GREEN_BD)

# ===== ③ 异步消息 =====
with g.subgraph(name='cluster_msg') as c:
    c.attr(
        label='  ③ 异步消息  NATS JetStream', style='dashed,rounded',
        color=BORDER, bgcolor=GROUP_BG,
        fontsize='12', fontcolor=LABEL_CLR,
        penwidth='1.2', labeljust='l', margin='16',
    )
    c.node('nats_publish', '发布 "order.created"', fillcolor=PURPLE, color=PURPLE_BD)
    c.node('delay_msg',    '延迟消息 30 min 超时检查',  fillcolor=AMBER,  color=AMBER_BD)

# ===== ④ 成团 / 超时 =====
with g.subgraph(name='cluster_result') as c:
    c.attr(
        label='  ④ 成团 / 超时处理', style='dashed,rounded',
        color=BORDER, bgcolor=GROUP_BG,
        fontsize='12', fontcolor=LABEL_CLR,
        penwidth='1.2', labeljust='l', margin='16',
    )
    c.node('timeout_check',  'Consumer 超时检查',  fillcolor=GRAY_FILL, color=GRAY_BD)
    c.node('check_count',    '人数达标？',
           fillcolor=AMBER, color=AMBER_BD, shape='diamond', width='1.6', height='0.7')
    c.node('success',        '成团确认 → 订单履行',
           fillcolor=GREEN, color=GREEN_BD)
    c.node('timeout_cancel', '超时取消 → 释放库存 + 退款',
           fillcolor=CORAL, color=CORAL_BD)

# ===== 参团分支 =====
g.node('join_group', '其他用户参团', fillcolor=BLUE, color=BLUE_BD)

# ===== 连接线 =====
g.edge('user', 'open_group', label='发起拼团')
g.edge('open_group', 'handler_validate')
g.edge('handler_validate', 'redis_deduct', label='验证通过')
g.edge('redis_deduct', 'create_order', label='预扣成功')
g.edge('create_order', 'order_status')
g.edge('order_status', 'nats_publish')
g.edge('nats_publish', 'delay_msg', label='注册延迟')

# 参团
g.edge('join_group', 'handler_validate', label='同样验证', style='dashed')
g.edge('order_status', 'join_group', label='分享链接', style='dashed', dir='back')

# 超时检查
g.edge('delay_msg', 'timeout_check', label='30 min 触发')
g.edge('timeout_check', 'check_count')
g.edge('check_count', 'success', label='是')
g.edge('check_count', 'timeout_cancel', label='否')

# 释放库存
g.edge('timeout_cancel', 'redis_deduct', label='INCR 回滚',
       style='dashed', constraint='false')

# ── 渲染 ─────────────────────────────────────────────
g.render(filename=os.path.join(output_dir, 'groupbuy-flow'), cleanup=True)
print(f"✅ 拼团购物链路图已生成: {output_dir}/groupbuy-flow.png")
