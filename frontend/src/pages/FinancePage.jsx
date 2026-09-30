import React, { useEffect, useState } from 'react'
import { useSearchParams } from 'react-router-dom'
import { api } from '../api.js'
import { Badge, DEPOSIT, STAGE, RESPONSIBILITY, VISIT_RESULT, Modal, useToast, Empty, Stat, Money } from '../components.jsx'

export default function FinancePage() {
  const [sp] = useSearchParams()
  const [filter, setFilter] = useState(sp.get('overdue') ? 'overdue' : '')
  const [data, setData] = useState(null)
  const [proof, setProof] = useState(null)
  const toast = useToast()
  const load = () => {
    const q = filter === 'overdue' ? '?overdue=1'
      : filter === 'frozen' ? '?frozen=1'
      : filter ? '?status=' + filter : ''
    api('/finance/deposits' + q).then(setData)
  }
  useEffect(load, [filter])

  const freezeAll = async () => {
    try {
      const d = await api('/finance/freeze-overdue', { method: 'POST', body: {} })
      toast(`已按桶编号冻结 ${d.count} 只逾期桶押金，并向店长发送催还提醒`)
      load()
    } catch (e) { toast(e.message, 'err') }
  }

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

  const judge = async (c, responsibility) => {
    const note = prompt('责任判定备注（可留空）：',
      responsibility === 'BAR' ? '司机已到店，酒吧未备好空桶'
        : responsibility === 'DRIVER' ? '酒吧已备货，司机未按约到店' : '')
    if (note === null) return
    try {
      const d = await api('/finance/responsibility/' + c.cycle_id,
        { method: 'POST', body: { responsibility, note } })
      toast(responsibility === 'BAR'
        ? '判定酒吧责任，押金维持冻结，可继续扣款结算'
        : `判定${responsibility === 'DRIVER' ? '司机' : '无'}责任，押金解除冻结，释放日重算为 ${d.deposit_release_date}`)
      load()
    } catch (e) { toast(e.message, 'err') }
  }

  if (!data) return <div className="muted">加载中…</div>
  const s = data.summary

  return (
    <div>
      <h1>押金财务</h1>
      <div className="grid cols-4">
        <Stat label="占用中押金（含冻结）" value={<Money v={s.held_amount} />} kind="warn" sub={`${s.held_count} 只桶`} />
        <Stat label="逾期冻结中" value={<Money v={s.frozen_amount || 0} />} kind={s.frozen_count ? 'alert' : 'ok'} sub={`${s.frozen_count || 0} 只待责任判定`} />
        <Stat label="累计释放押金" value={<Money v={s.released_total} />} kind="ok" />
        <Stat label="累计扣款押金" value={<Money v={s.deducted_total} />} kind="alert" />
      </div>

      <div className="toolbar" style={{ marginTop: 16 }}>
        <div className="f"><label>筛选</label>
          <select value={filter} onChange={e => setFilter(e.target.value)}>
            <option value="">全部周期</option>
            <option value="overdue">仅账期逾期（含冻结）</option>
            <option value="frozen">仅逾期冻结</option>
            <option value="HELD">押金占用中</option>
            <option value="FROZEN">冻结状态</option>
            <option value="RELEASED">已释放</option>
            <option value="DEDUCTED">已扣款</option>
            <option value="PARTIAL">部分扣款</option>
          </select></div>
        <button className="danger" onClick={freezeAll}>按桶编号冻结全部逾期桶押金</button>
      </div>

      <div className="tablewrap panel" style={{ padding: 0 }}>
        <table>
          <thead><tr>
            <th>桶编号</th><th>酒吧/城市</th><th>酒款/批次</th><th>账期</th>
            <th>押金</th><th>冻结/责任</th><th>释放/扣款</th><th>凭证与桶况</th><th>状态</th><th>按桶操作</th>
          </tr></thead>
          <tbody>
            {data.deposits.map(c => {
              const frozen = c.deposit_status === 'FROZEN'
              const needJudge = frozen && c.overdue_responsibility === 'PENDING'
              return (
                <tr key={c.cycle_id} style={frozen ? { background: '#fdf6f5' } : undefined}>
                  <td><code className="kcode">{c.keg_code}</code></td>
                  <td>{c.bar}<div className="small muted">{c.city}</div></td>
                  <td className="small">{c.style} {c.batch_no}</td>
                  <td className="small">{c.deposit_due_date}
                    {c.overdue_days > 0 && c.deposit_status !== 'RELEASED' &&
                      <div style={{ color: 'var(--danger)' }}>逾期 {c.overdue_days} 天</div>}</td>
                  <td><Money v={c.deposit_amount} /></td>
                  <td className="small">
                    {frozen
                      ? <><Badge map={DEPOSIT} value="FROZEN" />
                          <div className="muted" style={{ marginTop: 2 }}>{c.deposit_frozen_at}</div>
                          <div style={{ marginTop: 4 }}><Badge map={RESPONSIBILITY} value={c.overdue_responsibility} /></div>
                          {c.deposit_release_date && <div style={{ color: 'var(--ok)', marginTop: 2 }}>释放日重算 {c.deposit_release_date}</div>}
                        </>
                      : <span className="muted">—</span>}
                  </td>
                  <td className="small">
                    {c.deposit_status === 'HELD' ? '—'
                      : frozen ? '责任厘清后结算'
                      : <>释放 <b>¥{c.released_amount}</b><br />扣款 <b style={{ color: 'var(--danger)' }}>¥{c.deducted_amount}</b>
                        {c.deduct_reason && <div className="muted">{c.deduct_reason}</div>}
                      </>}
                  </td>
                  <td className="small">
                    {(c.proof_count > 0 || c.visit_result) &&
                      <button className="small ghost" onClick={() => api('/cycles/proofs/' + c.cycle_id).then(setProof)}>
                        查看凭证{`（照片${c.proof_count}${c.visit_result ? '·到场' : ''}）`}
                      </button>}
                    <div style={{ marginTop: 4 }}>
                      {c.visit_result && <span style={{ marginRight: 4 }}><Badge map={VISIT_RESULT} value={c.visit_result} /></span>}
                      {[c.late_return && '延迟归还', c.missing_scan && '司机漏扫', c.lost && '丢失', c.deformed && '变形',
                        (c.residue_level !== '无' && c.residue_level) && `残液:${c.residue_level}`,
                        (c.cap_status !== '已封盖' && c.cap_status) && c.cap_status]
                        .filter(Boolean).map((x, i) => <span key={i} className="badge red" style={{ margin: 2 }}>{x}</span>)}
                      {!c.late_return && !c.missing_scan && !c.lost && !c.deformed && !c.visit_result &&
                        c.residue_level === '无' && c.cap_status === '已封盖'
                        && <span className="muted">桶况完好</span>}
                    </div>
                  </td>
                  <td><Badge map={DEPOSIT} value={c.deposit_status} /><div className="small muted"><Badge map={STAGE} value={c.stage} /></div></td>
                  <td>
                    {needJudge
                      ? <div className="flags" style={{ flexDirection: 'column', alignItems: 'flex-start' }}>
                          <span className="small" style={{ color: 'var(--danger)' }}>请先判定逾期责任：</span>
                          <button className="small danger" onClick={() => judge(c, 'BAR')}>判酒吧责任（未备货）</button>
                          <button className="small" style={{ background: '#7a4a9a', borderColor: '#7a4a9a', color: '#fff' }}
                            onClick={() => judge(c, 'DRIVER')}>判司机责任（到店失败）</button>
                          <button className="small subtle" onClick={() => judge(c, 'CLEARED')}>无责/厘清</button>
                        </div>
                      : (c.deposit_status === 'HELD' || frozen)
                        ? <div className="flags" style={{ flexDirection: 'column', alignItems: 'flex-start' }}>
                            <button className="small green" style={{ background: 'var(--ok)', borderColor: 'var(--ok)' }}
                              onClick={() => settle(c, 'release')}>释放押金</button>
                            <button className="small subtle" onClick={() => settle(c, 'partial')}>部分扣款</button>
                            <button className="small danger" onClick={() => settle(c, 'deduct')}>全额扣款</button>
                          </div>
                        : <span className="muted small">{c.settled_at ? `已结算 ${c.settled_at}` : '已结算'}</span>}
                  </td>
                </tr>
              )
            })}
          </tbody>
        </table>
        {data.deposits.length === 0 && <Empty text="无符合条件的记录" />}
      </div>

      {proof && <ProofModal d={proof} onClose={() => setProof(null)} />}
    </div>
  )
}

function ProofModal({ d, onClose }) {
  return (
    <Modal wide title={`逾期责任凭证 · ${d.keg_code}`} onClose={onClose}>
      <h3>店长上传的"已打空未回收"现场照片（{d.proofs.length}）</h3>
      {d.proofs.length === 0 && <div className="muted small">店长暂未上传现场照片。</div>}
      <div className="grid cols-2">
        {d.proofs.map(p => (
          <div key={p.id} className="panel" style={{ margin: 0 }}>
            <img src={p.photo_data} alt="现场照片" style={{ maxWidth: '100%', borderRadius: 6, border: '1px solid #ddd0b8' }} />
            <div className="small" style={{ marginTop: 6 }}>{p.note || '（无说明）'}</div>
            <div className="small muted">{p.created_by} · {p.created_at}</div>
          </div>
        ))}
      </div>
      <h3 style={{ marginTop: 14 }}>司机到场记录（{d.visits.length}）</h3>
      {d.visits.length === 0 && <div className="muted small">无到场异常记录。</div>}
      {d.visits.map((v, i) => (
        <div key={i} className="notif" style={{ background: '#faf6ee' }}>
          <div className="row">
            <Badge map={VISIT_RESULT} value={v.result} />
            <span className="small muted">{v.driver} · {v.visited_at}</span>
          </div>
          {v.note && <div className="small" style={{ marginTop: 4 }}>{v.note}</div>}
        </div>
      ))}
      <div className="small muted" style={{ marginTop: 10 }}>
        判定规则：存在「酒吧未备好空桶」记录倾向酒吧责任（维持冻结可扣款）；
        存在「司机未到店」记录且酒吧已备货倾向司机责任（解除冻结，释放时间重新计算）。
      </div>
    </Modal>
  )
}
