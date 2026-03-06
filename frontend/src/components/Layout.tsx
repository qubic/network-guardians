import { Outlet } from 'react-router-dom'
import Header from './Header'
import Footer from './Footer'

export default function Layout() {
  return (
    <div className="flex min-h-screen flex-col bg-primary-80">
      <Header />
      <main className="mx-auto w-full max-w-7xl flex-1 px-4 py-8">
        <Outlet />
      </main>
      <Footer />
    </div>
  )
}
