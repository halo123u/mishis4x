import { Link } from 'react-router-dom'

const UserForm = (props) => {
  const handleSubmit = (event) => {
    event.preventDefault()
    props.submit(event.target.username.value, event.target.password.value)
  }

  return (
    <form onSubmit={handleSubmit} className="stack">
      <div className="stack sm">
        <label htmlFor="username">Username</label>
        <input type="text" name="username" id="username" />
      </div>
      <div className="stack sm">
        <label htmlFor="password">Password</label>
        <input type="password" name="password" id="password" />
      </div>

      <button type="submit">{props.buttonText}</button>
      <div>
        <Link to={`/sign-up`}>Create account</Link>
      </div>
    </form>
  )
}

export default UserForm
