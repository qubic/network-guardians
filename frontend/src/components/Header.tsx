import { Link, useLocation } from 'react-router-dom'
import TopBar from './TopBar'

const navItems = [
  { path: '/', label: 'Dashboard' },
  { path: '/nodes', label: 'Nodes' },
  { path: '/leaderboard', label: 'Leaderboard' },
  { path: '/map', label: 'Map' },
  { path: '/get-started', label: 'Get Started' }
]

// Qubic icon 
const QubicIcon = () => (
  <svg width="18" height="32" viewBox="0 0 14 26" fill="none" xmlns="http://www.w3.org/2000/svg">
    <path d="M5.25 2H0.75C0.335786 2 0 2.33579 0 2.75V19.25C0 19.6642 0.335786 20 0.75 20H5.25C5.66421 20 6 19.6642 6 19.25V2.75C6 2.33579 5.66421 2 5.25 2Z" fill="white"/>
    <path d="M13.25 2H8.75C8.33579 2 8 2.33579 8 2.75V25.25C8 25.6642 8.33579 26 8.75 26H13.25C13.6642 26 14 25.6642 14 25.25V2.75C14 2.33579 13.6642 2 13.25 2Z" fill="white"/>
  </svg>
)

export default function Header() {
  const location = useLocation()

  return (
    <>
      <TopBar />
      <header className="border-b border-primary-60">
        <div className="mx-auto flex h-[52px] max-w-7xl items-center justify-between px-4">
          <Link to="/" className="flex items-center gap-3">
            <QubicIcon />
            <span className="font-space text-[22px] font-medium leading-none">
              <span className="text-white">qubic</span>
              <span className="text-primary-30"> guardians</span>
            </span>
          </Link>

          <nav className="flex gap-2">
            {navItems.map((item) => {
              const isActive = location.pathname === item.path
              return (
                <Link
                  key={item.path}
                  to={item.path}
                  className={`rounded-lg px-3 py-1.5 text-13 font-medium transition-colors ${
                    isActive
                      ? 'bg-primary-60 text-primary-30'
                      : 'text-gray-50 hover:bg-primary-70 hover:text-white'
                  }`}
                >
                  {item.label}
                </Link>
              )
            })}
          </nav>
        </div>
      </header>
    </>
  )
}
