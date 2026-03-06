import { Routes, Route } from 'react-router-dom'
import Layout from './components/Layout'
import Dashboard from './pages/Dashboard'
import Nodes from './pages/Nodes'
import Leaderboard from './pages/Leaderboard'
import NodeDetail from './pages/NodeDetail'
import Map from './pages/Map'
import GetStarted from './pages/GetStarted'
import NotFound from './pages/NotFound'

export default function App() {
  return (
    <Routes>
      <Route path="/" element={<Layout />}>
        <Route index element={<Dashboard />} />
        <Route path="nodes" element={<Nodes />} />
        <Route path="nodes/:operator/:type" element={<NodeDetail />} />
        <Route path="leaderboard" element={<Leaderboard />} />
        <Route path="map" element={<Map />} />
        <Route path="get-started" element={<GetStarted />} />
        <Route path="*" element={<NotFound />} />
      </Route>
    </Routes>
  )
}
