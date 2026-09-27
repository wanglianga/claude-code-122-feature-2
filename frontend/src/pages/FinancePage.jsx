import React, { useEffect, useState } from 'react'
import { useSearchParams } from 'react-router-dom'
import { api } from '../api.js'
import { Badge, DEPOSIT, STAGE, useToast, Empty, Stat, Money } from '../components.jsx'

export default function FinancePage() {
  const [sp] = useSearchParams()
  const [filter, setFilter] = useState(sp.get('overdue') ? 'overdue' : '')
  const [data, setData] = useState(null)
  const [lostOpen, setLostOpen] = useState(null)
  const toast = useToast()
  const load = () => {
    const q = filter === 'overdue' ? '?overdue=1' : filter ? '?status=' + filter : ''
    api('/finance/deposits' + q).then(setData)
  }
  useEffect(load, [filter])

  const settle = async (c, action) => {
    let deduct = 0
    let reason = ''
    if (action === 'partial') {
      const v = prompt(`对 ${c.keg_code} 部分扣款金额（押金 ¥${c.deposit_amount}）：`)
      if (!v) return
      deduct = Number(v)
      reason = prompt('扣款原因：') || ''
    } else if (action === 'deduct') {
      reason = prompt('扣款原因：', '酒桶丢失/严重损坏，押金全额扣除') || ''
    }
    try {
      const d = await api('/finance/settle/' + c.cycle_id, { method: 'POST', body: { action, deduct_amount: deduct, reason } })
      toast(`结算完成：${d.deposit_status} 释放 ¥${d.released_amount} 扣款 ¥${d.deducted_amount}`)
      load()
    } catch (e) { toast(e.message, 'err') }
  }

  if (!data) return <div className="muted">加载中…</div>
  const s = data.summary

  return (
    <div>
      <h1>押金财务</h1>
      <div className="grid cols-4">
        <Stat label="占用中押金" value={<Money v={s.held_amount} />} kind="warn" sub={`${s.held_count} 只桶`} />
        <Stat label="其中逾期未闭环" value={s.overdue_count} kind={s.overdue_count ? 'alert' : 'ok'} />
        <Stat label="累计释放押金" value={<Money v={s.released_total} />} kind="ok" />
        <Stat label="累计扣款押金" value={<Money v={s.deducted_total} />} kind="alert" />
      </div>

      <div className="toolbar" style={{ marginTop: 16 }}>
        <div className="f"><label>筛选</label>
          <select value={filter} onChange={e => setFilter(e.target.value)}>
            <option value="">全部周期</option>
            <option value="overdue">仅账期逾期</option>
            <option value="HELD">押金占用中</option>
            <option value="RELEASED">已释放</option>
            <option value="DEDUCTED">已扣款</option>
            <option value="PARTIAL">部分扣款</option>
          </select></div>
      </div>

      <div className="tablewrap panel" style={{ padding: 0 }}>
        <table>
          <thead><tr>
            <th>桶编号</th><th>酒吧/城市</th><th>酒款/批次</th><th>账期</th>
            <th>押金</th><th>释放/扣款</th><th>桶况与异常</th><th>状态</th><th>按桶操作</th>
          </tr></thead>
          <tbody>
            {data.deposits.map(c => (
              <tr key={c.cycle_id}>
                <td><code className="kcode">{c.keg_code}</code></td>
                <td>{c.bar}<div className="small muted">{c.city}</div></td>
                <td className="small">{c.style} {c.batch_no}</td>
                <td className="small">{c.deposit_due_date}
                  {c.overdue_days > 0 && c.deposit_status === 'HELD' &&
                    <div style={{ color: 'var(--danger)' }}>逾期 {c.overdue_days} 天</div>}
                </td>
                <td><Money v={c.deposit_amount} /></td>
                <td className="small">
                  {c.deposit_status === 'HELD' ? '—'
                    : <>释放 <b>¥{c.released_amount}</b><br />扣款 <b style={{ color: 'var(--danger)' }}>¥{c.deducted_amount}</b>
                      {c.deduct_reason && <div className="muted">{c.deduct_reason}</div>}
                    </>}
                </td>
                <td className="small">
                  {[c.late_return && '延迟归还', c.missing_scan && '司机漏扫', c.lost && '丢失', c.deformed && '变形',
                    (c.residue_level !== '无' && c.residue_level) && `残液:${c.residue_level}`,
                    (c.cap_status !== '已封盖' && c.cap_status) && c.cap_status]
                    .filter(Boolean).map((x, i) => <span key={i} className="badge red" style={{ margin: 2 }}>{x}</span>)}
                  {!c.late_return && !c.missing_scan && !c.lost && !c.deformed && c.residue_level === '无' && c.cap_status === '已封盖'
                    && <span className="muted">桶况完好</span>}
                </td>
                <td><Badge map={DEPOSIT} value={c.deposit_status} /><div className="small muted"><Badge map={STAGE} value={c.stage} /></div></td>
                <td>
                  {c.deposit_status === 'HELD'
                    ? <div className="flags" style={{ flexDirection: 'column', alignItems: 'flex-start' }}>
                        <button className="small green" style={{ background: 'var(--ok)', borderColor: 'var(--ok)' }}
                          onClick={() => settle(c, 'release')}>释放押金</button>
                        <button className="small subtle" onClick={() => settle(c, 'partial')}>部分扣款</button>
                        <button className="small danger" onClick={() => settle(c, 'deduct')}>全额扣款</button>
                      </div>
                    : <span className="muted small">{c.settled_at ? `已结算 ${c.settled_at}` : '已结算'}</span>}
                </td>
              </tr>
            ))}
          </tbody>
        </table>
        {data.deposits.length === 0 && <Empty text="无符合条件的记录" />}
      </div>
    </div>
  )
}
