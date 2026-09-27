import React, { useEffect, useState } from 'react'
import { api } from '../api.js'
import { Badge, BATCH_STATUS, Modal, useToast, Empty } from '../components.jsx'

const ORDER_STATUS = { PLANNED: ['待配送', 'amber'], DELIVERING: ['配送中', 'blue'], DELIVERED: ['已送达', 'green'], CANCELLED: ['已取消', 'gray'] }

export default function Orders() {
  const [orders, setOrders] = useState([])
  const [opts, setOpts] = useState(null)
  const [createOpen, setCreateOpen] = useState(false)
  const toast = useToast()
  const load = () => api('/orders').then(d => setOrders(d.orders))
  useEffect(() => { load(); api('/options').then(setOpts) }, [])

  const dispatch = async (o) => {
    const driverId = prompt(`为订单 ${o.order_no} 指定配送司机 ID（1=王师傅/上海, 2=李师傅/北京）：`, o.driver === '未指派' ? '1' : '1')
    if (!driverId) return
    const receiver = prompt('酒吧收货人：', '值班店长')
    if (!receiver) return
    try {
      const d = await api('/orders/dispatch/' + o.id, {
        method: 'POST', body: { driver_id: Number(driverId), bar_receiver: receiver }
      })
      toast(`已发货 ${d.dispatched.length} 只：${d.dispatched.map(x => x.keg_code).join('、')}`)
      load()
    } catch (e) { toast(e.message, 'err') }
  }

  return (
    <div>
      <div className="row" style={{ marginBottom: 14 }}>
        <h1 style={{ margin: 0 }}>订单与发货</h1>
        <span className="spacer" />
        <button className="ghost" onClick={() => setCreateOpen(true)}>新建订单（支持节日铺货）</button>
      </div>

      <div className="tablewrap panel" style={{ padding: 0 }}>
        <table>
          <thead><tr>
            <th>订单号</th><th>酒吧/城市</th><th>经销商</th><th>司机</th><th>明细（批次×数量）</th>
            <th>账期</th><th>标记</th><th>状态</th><th>操作</th>
          </tr></thead>
          <tbody>
            {orders.map(o => (
              <tr key={o.id}>
                <td><b>{o.order_no}</b><div className="small muted">{o.delivered_at ? `送达 ${o.delivered_at}` : `建单 ${o.created_at}`}</div></td>
                <td>{o.bar}<div className="small muted">{o.city}</div></td>
                <td>{o.dealer}</td>
                <td>{o.driver}</td>
                <td>
                  {o.items.map((it, i) => (
                    <div key={i} className="small">
                      {it.beer}（{it.style}）<span className="muted"> {it.batch_no}</span> ×{it.qty}
                      {it.batch_status !== 'NORMAL' && <span className="badge red" style={{ marginLeft: 4 }}>批次已停售</span>}
                    </div>
                  ))}
                </td>
                <td>{o.deposit_term_days} 天</td>
                <td>{o.is_festival ? <span className="badge purple">节日铺货</span> : '常规'}</td>
                <td><Badge map={ORDER_STATUS} value={o.status} /></td>
                <td>
                  {o.status === 'PLANNED'
                    ? <button className="small" onClick={() => dispatch(o)}>发货扫码</button>
                    : <span className="muted small">—</span>}
                </td>
              </tr>
            ))}
          </tbody>
        </table>
        {orders.length === 0 && <Empty />}
      </div>

      {createOpen && opts && <CreateOrder opts={opts} onClose={() => setCreateOpen(false)} onDone={() => { setCreateOpen(false); load() }} />}
    </div>
  )
}

function CreateOrder({ opts, onClose, onDone }) {
  const [barId, setBarId] = useState(opts.bars[0]?.id || '')
  const [driverId, setDriverId] = useState('')
  const [term, setTerm] = useState(30)
  const [festival, setFestival] = useState(false)
  const [note, setNote] = useState('')
  const [items, setItems] = useState([{ batch_id: '', qty: 1 }])
  const toast = useToast()
  const bar = opts.bars.find(b => b.id === barId)

  const submit = async () => {
    try {
      await api('/orders', {
        method: 'POST',
        body: {
          bar_id: Number(barId), driver_id: Number(driverId) || 0,
          is_festival: festival, festival_note: note, deposit_term_days: Number(term),
          items: items.filter(x => x.batch_id).map(x => ({ batch_id: Number(x.batch_id), qty: Number(x.qty) }))
        }
      })
      toast('订单已创建（待配送）')
      onDone()
    } catch (e) { toast(e.message, 'err') }
  }

  return (
    <Modal wide title="新建配送订单" onClose={onClose}>
      <div className="grid cols-3">
        <div><label>收货酒吧</label>
          <select value={barId} onChange={e => setBarId(Number(e.target.value))}>
            {opts.bars.map(b => <option key={b.id} value={b.id}>{b.city} · {b.name}</option>)}
          </select></div>
        <div><label>配送司机（可后补）</label>
          <select value={driverId} onChange={e => setDriverId(e.target.value)}>
            <option value="">暂不指派</option>
            {opts.drivers.map(d => <option key={d.id} value={d.id}>{d.city} · {d.name}</option>)}
          </select></div>
        <div><label>押金账期（天）</label><input type="number" value={term} onChange={e => setTerm(e.target.value)} /></div>
      </div>
      <div className="checks" style={{ marginTop: 12 }}>
        <label><input type="checkbox" checked={festival} onChange={e => setFestival(e.target.checked)} /> 节日临时铺货（节后统一回收）</label>
      </div>
      {festival && <div style={{ marginTop: 8 }}><label>铺货说明</label>
        <input value={note} onChange={e => setNote(e.target.value)} placeholder="如：中秋国庆双节临时加配" /></div>}

      <div className="section-title">配送明细</div>
      {items.map((it, i) => (
        <div className="row" key={i} style={{ marginBottom: 8 }}>
          <select style={{ flex: 2 }} value={it.batch_id} onChange={e => {
            const cp = [...items]; cp[i].batch_id = e.target.value; setItems(cp) }}>
            <option value="">选择批次</option>
            {opts.batches.map(b => <option key={b.id} value={b.id}>{b.beer}（{b.style}）{b.batch_no} {b.status !== 'NORMAL' ? '[' + b.status + ']' : ''}</option>)}
          </select>
          <input style={{ width: 110 }} type="number" min={1} value={it.qty} onChange={e => {
            const cp = [...items]; cp[i].qty = e.target.value; setItems(cp) }} />
          <button className="subtle small" onClick={() => setItems(items.filter((_, j) => j !== i))}>删除</button>
        </div>
      ))}
      <button className="ghost small" onClick={() => setItems([...items, { batch_id: '', qty: 1 }])}>+ 添加明细行</button>

      <div className="row end" style={{ marginTop: 16 }}>
        <button className="subtle" onClick={onClose}>取消</button>
        <button onClick={submit}>创建订单</button>
      </div>
    </Modal>
  )
}
