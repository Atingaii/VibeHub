#!/usr/bin/env python3
"""VibeShop 部署架构图 — Docker Compose — 极简柔和风格"""

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
g = graphviz.Digraph('deploy', format='png')
g.attr(
    rankdir='TB', dpi='200', bgcolor=BG, pad='0.8',
    fontname=FONT, nodesep='0.6', ranksep='0.8',
    margin='0.3',
)
g.attr('node',
    fontname=FONT, fontsize='11', fontcolor=TEXT_CLR,
    style='filled,rounded', shape='box',
    penwidth='1.0', height='0.6',
)
g.attr('edge',
    fontname=FONT, fontsize='9', fontcolor='#667085',
    color=EDGE_CLR, arrowsize='0.7', penwidth='0.9',
)

# ===== 外部访问 =====
g.node('client', '客户端\nWeb / Mobile / MCP', shape='ellipse',
       fillcolor=BLUE, color=BLUE_BD, width='2.0', height='0.6')

# ===== Docker Compose =====
with g.subgraph(name='cluster_docker') as d:
    d.attr(
        label='  Docker Compose  开发 / 演示环境', style='dashed,rounded',
        color=BORDER, bgcolor=GROUP_BG,
        fontsize='13', fontcolor=LABEL_CLR,
        penwidth='1.2', labeljust='l', margin='20',
    )

    # 应用容器
    with d.subgraph(name='cluster_app') as a:
        a.attr(
            label='  应用容器', style='dashed,rounded',
            color='#E4E7EC', bgcolor='#F9FAFB',
            fontsize='11', fontcolor='#667085',
            penwidth='0.8', margin='16',
        )
        a.node('vibeshop', 'vibeshop\nAlpine ~15MB · :8080',
               fillcolor=GREEN, color=GREEN_BD,
               fontsize='12', width='2.5', height='0.7')

    # 基础设施容器
    with d.subgraph(name='cluster_infra') as i:
        i.attr(
            label='  基础设施容器  docker-compose.infra.yml', style='dashed,rounded',
            color='#E4E7EC', bgcolor='#F9FAFB',
            fontsize='11', fontcolor='#667085',
            penwidth='0.8', margin='16',
        )
        i.node('mysql_c',  'MySQL 8.0+\n:3306',
               fillcolor=GRAY_FILL, color=GRAY_BD, shape='cylinder', width='1.5')
        i.node('pg_c',     'PostgreSQL 16\n:5432',
               fillcolor=GRAY_FILL, color=GRAY_BD, shape='cylinder', width='1.5')
        i.node('redis_c',  'Redis 7.x\n:6379',
               fillcolor=GRAY_FILL, color=GRAY_BD, shape='cylinder', width='1.5')
        i.node('nats_c',   'NATS\n:4222 / :8222',
               fillcolor=GRAY_FILL, color=GRAY_BD, shape='cylinder', width='1.5')
        i.node('jaeger_c', 'Jaeger\n:16686',
               fillcolor=GRAY_FILL, color=GRAY_BD, shape='cylinder', width='1.5')

# ===== 启动方式 =====
with g.subgraph(name='cluster_commands') as c:
    c.attr(
        label='  启动方式', style='dashed,rounded',
        color=BORDER, bgcolor=GROUP_BG,
        fontsize='12', fontcolor=LABEL_CLR,
        penwidth='1.2', labeljust='l', margin='16',
    )
    c.node('cmd_dev',    '开发热重载\nmake infra-up && make dev',
           fillcolor=GREEN, color=GREEN_BD, fontsize='9', width='2.2')
    c.node('cmd_quick',  '快捷启动\nmake quick-start',
           fillcolor=BLUE,  color=BLUE_BD,  fontsize='9', width='2.2')
    c.node('cmd_docker', '全 Docker\nmake docker-up',
           fillcolor=AMBER, color=AMBER_BD, fontsize='9', width='2.2')
    c.node('cmd_binary', '单二进制\nmake build → ./bin/vibeshop',
           fillcolor=GRAY_FILL, color=GRAY_BD, fontsize='9', width='2.2')

# ===== 连接 =====
g.edge('client', 'vibeshop', label='HTTP :8080')

g.edge('vibeshop', 'mysql_c',  label='TCP', penwidth='0.7')
g.edge('vibeshop', 'pg_c',     label='TCP', penwidth='0.7')
g.edge('vibeshop', 'redis_c',  label='TCP', penwidth='0.7')
g.edge('vibeshop', 'nats_c',   label='TCP', penwidth='0.7')
g.edge('vibeshop', 'jaeger_c', label='UDP', style='dashed', penwidth='0.7')

# 对齐中间件
with g.subgraph() as s:
    s.attr(rank='same')
    s.node('mysql_c'); s.node('pg_c'); s.node('redis_c')
    s.node('nats_c');  s.node('jaeger_c')

# 对齐启动命令
with g.subgraph() as s:
    s.attr(rank='same')
    s.node('cmd_dev'); s.node('cmd_quick'); s.node('cmd_docker'); s.node('cmd_binary')

# ── 渲染 ─────────────────────────────────────────────
g.render(filename=os.path.join(output_dir, 'deploy'), cleanup=True)
print(f"✅ 部署架构图已生成: {output_dir}/deploy.png")
