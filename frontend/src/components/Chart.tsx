import React from 'react'
import { LineChart, Line, XAxis, YAxis, Tooltip, ResponsiveContainer } from 'recharts'

const data = Array.from({length: 8}).map((_, i) => ({name: `T${i+1}`, value: Math.round(Math.random()*100)}))

export default function Chart(){
  return (
    <div className="w-full h-64 bg-white p-4 rounded shadow">
      <ResponsiveContainer width="100%" height="100%">
        <LineChart data={data}>
          <XAxis dataKey="name" />
          <YAxis />
          <Tooltip />
          <Line type="monotone" dataKey="value" stroke="#3b82f6" strokeWidth={2} />
        </LineChart>
      </ResponsiveContainer>
    </div>
  )
}
