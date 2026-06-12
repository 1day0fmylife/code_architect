import React from 'react'
import { useStore } from './store'
import Chart from './components/Chart'

export default function App(){
  const count = useStore(state => state.count)
  const inc = useStore(state => state.increment)
  return (
    <div className="min-h-screen bg-gray-50 p-6">
      <div className="max-w-4xl mx-auto">
        <h1 className="text-2xl font-bold mb-4">Code Archestrator Dashboard</h1>
        <button className="px-4 py-2 bg-blue-600 text-white rounded" onClick={inc}>Increment ({count})</button>
        <div className="mt-6">
          <Chart />
        </div>
      </div>
    </div>
  )
}
