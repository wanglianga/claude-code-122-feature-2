import React, { useEffect, useState } from 'react'
import { api, getUser } from '../api.js'
import { AUDIENCE, Badge, useToast, Empty } from '../components.jsx'

const AUD_BADGE = { BAR: 'red', PENDING_ORDER: 'amber', DEALER_STOCK: 'purple', ALL: 'blue' }
const ALERT_META = {
  OVERDUE_FREEZE: ['押金冻结', 'red'], RETURN_REMINDER: ['回收/到场提醒', 'amber'],
  LIABILITY_RESULT: ['责任结果', 'purple'], PHOTO_REVIEWED: ['照片核实', 'blue']
}

export default function NotificationsPage() {
  const user = getUser()
  const [list, setList] = useState([])
  const [alerts, setAlerts] = useState([])
  const toast = useToast()
  const load = () => {
    api('/notifications').then(d => setList(d.notifications))
    if (user && (user.role === 'bar_manager' || user.role === 'admin')) {
      api('/bar/alerts').then(d => setAlerts(d.alerts || [])).catch(() => {})
    }
  }
  useEffect(load, [])

  const ack = async (id) => {
    await api('/notifications/ack/' + id, { method: 'POST' })
    toast('已确认')
    load()
  }
  const ackAlert = async (id) => {
    await api('/bar/alerts/ack/' + id, { method: 'POST' })
    toast('已知悉')
    load()
  }

  return (
    <div>
      <h1>通知中心</h1>
      <div className="muted small" style={{ marginBottom: 12 }}>
        批次停售/召回按角色定向通知；空桶逾期时平台按桶冻结押金并向店长推送提醒，司机到店失败、酒吧未备好空桶、现场照片核实结果与逾期责任判定都会在此留痕。
      </div>

      {alerts.length > 0 && (
        <div className="panel" style={{ background: '#fffdf7', marginBottom: 14 }}>
          <h3 style={{ marginTop: 0 }}>逾期押金 / 回收提醒</h3>
          {alerts.map(a => (
            <div key={a.id} className={'notif ' + (a.acked ? 'read' : '')}>
              <div className="row">
                <span className={'badge ' + (ALERT_META[a.kind]?.[1] || 'gray')}>{ALERT_META[a.kind]?.[0] || a.kind}</span>
                {a.keg_code && <code className="kcode">{a.keg_code}</code>}
                <span className="spacer" />
                <span className="small muted">{a.created_at}</span>
                {!a.acked && <button className="small ghost" onClick={() => ackAlert(a.id)}>确认已知悉</button>}
              </div>
              <div className="tt" style={{ marginTop: 6 }}>{a.title}</div>
              <div className="small">{a.body}</div>
            </div>
          ))}
        </div>
      )}

      <div className="panel" style={{ background: '#fffdf7' }}>
        <h3 style={{ marginTop: 0 }}>批次停售 / 召回通知</h3>
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
        {list.length === 0 && alerts.length === 0 && <Empty text="暂无通知" />}
      </div>
    </div>
  )
}
