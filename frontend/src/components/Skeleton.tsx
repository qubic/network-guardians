interface SkeletonProps {
  className?: string
}

export default function Skeleton({ className = '' }: SkeletonProps) {
  return (
    <div
      className={`animate-pulse rounded-lg bg-primary-60 ${className}`}
    />
  )
}
