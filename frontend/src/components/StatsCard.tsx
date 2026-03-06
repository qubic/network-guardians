import { ReactNode } from 'react'
import Card from './Card'

interface StatsCardProps {
  label: string
  value: string | number
  icon?: ReactNode
  subValue?: string
}

export default function StatsCard({ label, value, icon, subValue }: StatsCardProps) {
  return (
    <Card className="p-4">
      <div className="flex items-center gap-4">
        {icon && (
          <div className="flex h-10 w-10 shrink-0 items-center justify-center rounded-lg bg-primary-60 text-primary-30">
            {icon}
          </div>
        )}
        <div className="min-w-0 flex-1">
          <p className="text-13 text-gray-50">{label}</p>
          <p className="mt-1 truncate font-space text-20 font-semibold text-white">
            {value}
          </p>
          {subValue && (
            <p className="mt-0.5 font-space text-11 tracking-wide text-gray-50">{subValue}</p>
          )}
        </div>
      </div>
    </Card>
  )
}
