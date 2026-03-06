import { Link } from 'react-router-dom'
import Card from '../components/Card'

export default function NotFound() {
  return (
    <div className="flex min-h-[60vh] items-center justify-center">
      <Card className="max-w-md p-8 text-center">
        <div className="mb-4 text-6xl font-bold text-primary-30">404</div>
        <h1 className="mb-2 font-space text-xl font-semibold text-white">
          Page Not Found
        </h1>
        <p className="mb-6 text-gray-50">
          The page you're looking for doesn't exist or has been moved.
        </p>
        <Link
          to="/"
          className="inline-block rounded-lg bg-primary-50 px-6 py-2.5 text-sm font-medium text-white transition-colors hover:bg-primary-40"
        >
          Back to Dashboard
        </Link>
      </Card>
    </div>
  )
}
