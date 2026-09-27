import React, { useEffect, useState } from 'react'
import { api } from '../api.js'
import { AUDIENCE, Badge, useToast, Empty } from '../components.jsx'

const AUD_BADGE = { BAR: 'red', PENDING_ORDER: 'amber', DEALER_STOCK: 'purple', ALL: 'blue' }

export default function NotificationsPage() {
  const [list, setList] = useState([])
  const toast = useToast()
  const load = () => api('/notifications').then(d => setList(d.notifications))
  useEffect(load, [])

  const ack = async (id) => {
    await api('/notifications/ack/' + id, { method: 'POST' })
    toast('已确认')
    load()
  }

  return (
    <div>
      <h1>批次停售 / 召回通知</h1>
      <div className="muted small" style={{ marginBottom: 12 }}>
        批次暂停销售或召回时，系统自动通知<b>已收货酒吧</b>（区分已开/未开桶）、<b>待配送订单</b>与<b>经销商库存</b>，并附全渠道广播，防止继续开桶售卖。
      </div>
      <div className="panel" style={{ background: '#fffdf7' }}>
        {list.map(n => (
          <div key={n.id} className={'notif ' + (n.acked ? 'read' : '')}>
            <div className="row">
              <span className={'badge ' + (AUD_BADGE[n.audience] || 'gray')}>{AUDIENCE[n.audience]}</span>
              <span className="small muted">{n.style} · {n.batch_no}</span>
              <span className="spacer" />
              <span className="small muted">{n.created_at}</span>
              {!n.acked && <button className="small ghost" onClick={() => ack(n.id)}>确认已知悉</button>}
            </div>
            <div className="tt" style={{ marginTop: 6 }}>{n.title}</div>
            <div className="small">{n.body}</div>
          </div>
        ))}
        {list.length === 0 && <Empty text="暂无通知" />}
      </div>
    </div>
  )
}
